package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/metril/speedtest-tracker/internal/runner"
	"github.com/metril/speedtest-tracker/internal/scheduler"
	"github.com/metril/speedtest-tracker/internal/store"
)

// nextRunCount is how many upcoming fire times the UI previews.
const nextRunCount = 5

// scheduleBody is the request payload for schedule create/update.
type scheduleBody struct {
	Name      string  `json:"name"`
	Cron      string  `json:"cron"`
	Enabled   bool    `json:"enabled"`
	Timezone  string  `json:"timezone"`
	TargetIDs []int64 `json:"target_ids"`
}

// scheduleView is a stored schedule plus its computed next fire time
// (empty when the schedule is disabled or its expression cannot fire).
type scheduleView struct {
	store.Schedule
	NextRun string `json:"next_run"`
}

// viewSchedule computes next_run: from the running scheduler when the
// schedule is registered, otherwise by parsing its expression.
func (d Deps) viewSchedule(sc store.Schedule, now time.Time) scheduleView {
	v := scheduleView{Schedule: sc}
	if !sc.Enabled {
		return v
	}
	if d.Scheduler != nil {
		if at, ok := d.Scheduler.Next(sc.ID); ok {
			v.NextRun = at.Format(time.RFC3339)
			return v
		}
	}
	times, err := scheduler.NextFireTimes(sc.Cron, sc.Timezone, 1, now)
	if err == nil && len(times) > 0 {
		v.NextRun = times[0].Format(time.RFC3339)
	}
	return v
}

// validateSchedule normalises and checks a schedule payload, answering 400
// itself. It reports whether the payload is usable.
func (d Deps) validateSchedule(w http.ResponseWriter, r *http.Request, b *scheduleBody) bool {
	b.Name = strings.TrimSpace(b.Name)
	if b.Name == "" {
		errBadRequest(w, "name is required")
		return false
	}
	b.Cron = strings.TrimSpace(b.Cron)
	if err := scheduler.ValidateCron(b.Cron); err != nil {
		errBadRequest(w, err.Error())
		return false
	}
	if strings.TrimSpace(b.Timezone) == "" {
		b.Timezone = "UTC"
	}
	if _, err := time.LoadLocation(b.Timezone); err != nil {
		errBadRequest(w, "unknown timezone "+b.Timezone)
		return false
	}
	if len(b.TargetIDs) == 0 {
		errBadRequest(w, "target_ids must contain at least one target")
		return false
	}
	seen := make(map[int64]bool, len(b.TargetIDs))
	for _, id := range b.TargetIDs {
		if seen[id] {
			errBadRequest(w, "target_ids contains duplicates")
			return false
		}
		seen[id] = true
	}
	found, err := d.Store.ListTargetsByIDs(r.Context(), b.TargetIDs)
	if err != nil {
		internalError(w, d.Logger, "load targets failed", err)
		return false
	}
	if len(found) != len(b.TargetIDs) {
		errBadRequest(w, "target_ids contains an unknown target")
		return false
	}
	return true
}

// reloadTimeout bounds a reload triggered from an HTTP handler; it runs
// detached from the request context so a client disconnect can't abort it
// and leave the cron stale.
const reloadTimeout = 10 * time.Second

// reloadSchedules notifies the scheduler that stored schedules changed. It
// runs with a fresh timeout detached from r's cancellation so a client
// disconnecting mid-request never aborts the reload.
func (d Deps) reloadSchedules(ctx context.Context) {
	if d.ReloadSchedules == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), reloadTimeout)
	defer cancel()
	if err := d.ReloadSchedules(ctx); err != nil {
		d.Logger.Error("reload schedules", "error", err)
	}
}

// scheduleStoreError maps store errors for schedules onto status codes.
// It reports whether it handled err.
func (d Deps) scheduleStoreError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, store.ErrNameConflict) {
		writeError(w, http.StatusConflict, "name_conflict", "a schedule with that name already exists")
		return true
	}
	return storeError(w, d.Logger, "schedule", err)
}

func (d Deps) listSchedules(w http.ResponseWriter, r *http.Request) {
	list, err := d.Store.ListSchedules(r.Context())
	if err != nil {
		internalError(w, d.Logger, "list schedules failed", err)
		return
	}
	now := time.Now()
	views := make([]scheduleView, 0, len(list))
	for _, sc := range list {
		views = append(views, d.viewSchedule(sc, now))
	}
	writeJSON(w, http.StatusOK, map[string]any{"schedules": views})
}

func (d Deps) getSchedule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	sc, err := d.Store.GetSchedule(r.Context(), id)
	if d.scheduleStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, d.viewSchedule(*sc, time.Now()))
}

func (d Deps) createSchedule(w http.ResponseWriter, r *http.Request) {
	var b scheduleBody
	if !decodeJSON(w, r, &b) || !d.validateSchedule(w, r, &b) {
		return
	}
	sc := &store.Schedule{Name: b.Name, Cron: b.Cron, Enabled: b.Enabled,
		Timezone: b.Timezone, TargetIDs: b.TargetIDs}
	id, err := d.Store.CreateSchedule(r.Context(), sc)
	if d.scheduleStoreError(w, err) {
		return
	}
	d.reloadSchedules(r.Context())
	d.respondSchedule(w, r, id, http.StatusCreated)
}

func (d Deps) updateSchedule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var b scheduleBody
	if !decodeJSON(w, r, &b) || !d.validateSchedule(w, r, &b) {
		return
	}
	sc := &store.Schedule{ID: id, Name: b.Name, Cron: b.Cron, Enabled: b.Enabled,
		Timezone: b.Timezone, TargetIDs: b.TargetIDs}
	if err := d.Store.UpdateSchedule(r.Context(), sc); d.scheduleStoreError(w, err) {
		return
	}
	d.reloadSchedules(r.Context())
	d.respondSchedule(w, r, id, http.StatusOK)
}

// respondSchedule re-reads the saved schedule and answers with it plus any
// overlap warnings (Task 5 fills warnings in; until then it is empty).
func (d Deps) respondSchedule(w http.ResponseWriter, r *http.Request, id int64, status int) {
	saved, err := d.Store.GetSchedule(r.Context(), id)
	if d.scheduleStoreError(w, err) {
		return
	}
	writeJSON(w, status, map[string]any{
		"schedule": d.viewSchedule(*saved, time.Now()),
		"warnings": d.overlapWarnings(r.Context(), *saved),
	})
}

func (d Deps) deleteSchedule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := d.Store.DeleteSchedule(r.Context(), id); d.scheduleStoreError(w, err) {
		return
	}
	d.reloadSchedules(r.Context())
	w.WriteHeader(http.StatusNoContent)
}

// validateCron answers POST /schedules/validate: 200 with the next fire
// times, or 400 with the parse error.
func (d Deps) validateCron(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Cron     string `json:"cron"`
		Timezone string `json:"timezone"`
	}
	if !decodeJSON(w, r, &b) {
		return
	}
	times, err := scheduler.NextFireTimes(strings.TrimSpace(b.Cron), b.Timezone, nextRunCount, time.Now())
	if err != nil {
		errBadRequest(w, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "next": formatTimes(times)})
}

// formatTimes renders fire times as RFC3339 strings.
func formatTimes(times []time.Time) []string {
	out := make([]string, 0, len(times))
	for _, t := range times {
		out = append(out, t.Format(time.RFC3339))
	}
	return out
}

// overlapWarnings reports every other enabled schedule that shares a queue
// with sc and fires within 60s of it in the next 24h. Overlapping tests on
// one queue skew each other's numbers, so the UI shows these on save. A
// disabled schedule cannot collide with anything.
func (d Deps) overlapWarnings(ctx context.Context, sc store.Schedule) []string {
	if !sc.Enabled {
		return []string{}
	}
	all, err := d.Store.ListSchedules(ctx)
	if err != nil {
		d.Logger.Error("overlap check: list schedules", "error", err)
		return []string{}
	}
	targets, err := d.Store.ListTargets(ctx)
	if err != nil {
		d.Logger.Error("overlap check: list targets", "error", err)
		return []string{}
	}
	queueOf := make(map[int64]string, len(targets))
	for _, t := range targets {
		queueOf[t.ID] = t.QueueName
	}
	candidate := func(s store.Schedule) scheduler.OverlapCandidate {
		seen := map[string]bool{}
		queues := []string{}
		for _, tid := range s.TargetIDs {
			if queue, ok := queueOf[tid]; ok && !seen[queue] {
				seen[queue] = true
				queues = append(queues, queue)
			}
		}
		return scheduler.OverlapCandidate{ID: s.ID, Name: s.Name, Cron: s.Cron,
			Timezone: s.Timezone, Queues: queues}
	}
	others := make([]scheduler.OverlapCandidate, 0, len(all))
	for _, other := range all {
		if other.ID == sc.ID || !other.Enabled {
			continue
		}
		others = append(others, candidate(other))
	}
	warnings := scheduler.FindOverlaps(candidate(sc), others, time.Now())
	if warnings == nil {
		return []string{}
	}
	return warnings
}

// runSchedule answers POST /schedules/{id}/run: a manual, immediate run of
// the schedule's ordered targets, still tagged with the schedule id so the
// run shows up in that schedule's history.
func (d Deps) runSchedule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	sc, err := d.Store.GetSchedule(r.Context(), id)
	if d.scheduleStoreError(w, err) {
		return
	}
	runID, err := d.Runner.Enqueue(r.Context(), runner.RunRequest{
		Trigger: "manual", ScheduleID: &sc.ID, TargetIDs: sc.TargetIDs})
	if err != nil {
		enqueueError(w, d.Logger, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]int64{"run_id": runID})
}

// scheduleNext answers GET /schedules/{id}/next with the next five fire
// times of the stored expression.
func (d Deps) scheduleNext(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	sc, err := d.Store.GetSchedule(r.Context(), id)
	if d.scheduleStoreError(w, err) {
		return
	}
	if !sc.Enabled {
		writeJSON(w, http.StatusOK, map[string]any{"next": []string{}})
		return
	}
	times, err := scheduler.NextFireTimes(sc.Cron, sc.Timezone, nextRunCount, time.Now())
	if err != nil {
		errBadRequest(w, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"next": formatTimes(times)})
}
