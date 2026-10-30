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
func (d Deps) validateSchedule(ctx context.Context, w http.ResponseWriter, b *scheduleBody) bool {
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
	found, err := d.Store.ListTargetsByIDs(ctx, b.TargetIDs)
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

func (d Deps) listSchedules(ctx context.Context, _ *struct{}) (*jsonOut[ScheduleList], error) {
	list, err := d.Store.ListSchedules(ctx)
	if err != nil {
		return nil, internalErr(d.Logger, "list schedules failed", err)
	}
	now := time.Now()
	views := make([]scheduleView, 0, len(list))
	for _, sc := range list {
		views = append(views, d.viewSchedule(sc, now))
	}
	return &jsonOut[ScheduleList]{ScheduleList{Schedules: views}}, nil
}

// ScheduleList is the GET /schedules body.
type ScheduleList struct {
	Schedules []scheduleView `json:"schedules"`
}

// ScheduleSaved is the schedule create/update response.
type ScheduleSaved struct {
	Schedule scheduleView `json:"schedule"`
	Warnings []string     `json:"warnings" doc:"Overlap warnings computed for the saved schedule."`
}

// CronValidateRequest is the POST /schedules/validate body.
type CronValidateRequest struct {
	Cron     string `json:"cron"`
	Timezone string `json:"timezone"`
}

// CronValidateResponse previews the next fire times of a valid expression.
type CronValidateResponse struct {
	Next []string `json:"next"`
	OK   bool     `json:"ok"`
}

// ScheduleNext is the GET /schedules/{id}/next body.
type ScheduleNext struct {
	Next []string `json:"next"`
}

type scheduleIn struct {
	ID   int64 `path:"id" minimum:"1" doc:"Resource ID."`
	Body scheduleBody
}

type scheduleCreateIn struct {
	Body scheduleBody
}

func (d Deps) scheduleStoreErr(err error) error {
	if err == nil {
		return nil
	}
	c := &errCapture{}
	d.scheduleStoreError(c, err)
	return c.err()
}

func (d Deps) getSchedule(ctx context.Context, in *idIn) (*jsonOut[scheduleView], error) {
	if err := in.check(); err != nil {
		return nil, err
	}
	sc, err := d.Store.GetSchedule(ctx, in.ID)
	if e := d.scheduleStoreErr(err); e != nil {
		return nil, e
	}
	return &jsonOut[scheduleView]{d.viewSchedule(*sc, time.Now())}, nil
}

func (d Deps) createSchedule(ctx context.Context, in *scheduleCreateIn) (*jsonOut[ScheduleSaved], error) {
	b := in.Body
	c := &errCapture{}
	if !d.validateSchedule(ctx, c, &b) {
		return nil, c.err()
	}
	sc := &store.Schedule{Name: b.Name, Cron: b.Cron, Enabled: b.Enabled,
		Timezone: b.Timezone, TargetIDs: b.TargetIDs}
	id, err := d.Store.CreateSchedule(ctx, sc)
	if e := d.scheduleStoreErr(err); e != nil {
		return nil, e
	}
	d.reloadSchedules(ctx)
	return d.respondSchedule(ctx, id)
}

func (d Deps) updateSchedule(ctx context.Context, in *scheduleIn) (*jsonOut[ScheduleSaved], error) {
	if err := (idIn{in.ID}).check(); err != nil {
		return nil, err
	}
	b := in.Body
	c := &errCapture{}
	if !d.validateSchedule(ctx, c, &b) {
		return nil, c.err()
	}
	sc := &store.Schedule{ID: in.ID, Name: b.Name, Cron: b.Cron, Enabled: b.Enabled,
		Timezone: b.Timezone, TargetIDs: b.TargetIDs}
	if err := d.Store.UpdateSchedule(ctx, sc); err != nil {
		return nil, d.scheduleStoreErr(err)
	}
	d.reloadSchedules(ctx)
	return d.respondSchedule(ctx, in.ID)
}

// respondSchedule re-reads the saved schedule and answers with it plus any
// overlap warnings (Task 5 fills warnings in; until then it is empty).
func (d Deps) respondSchedule(ctx context.Context, id int64) (*jsonOut[ScheduleSaved], error) {
	saved, err := d.Store.GetSchedule(ctx, id)
	if e := d.scheduleStoreErr(err); e != nil {
		return nil, e
	}
	return &jsonOut[ScheduleSaved]{ScheduleSaved{
		Schedule: d.viewSchedule(*saved, time.Now()),
		Warnings: d.overlapWarnings(ctx, *saved),
	}}, nil
}

func (d Deps) deleteSchedule(ctx context.Context, in *idIn) (*struct{}, error) {
	if err := in.check(); err != nil {
		return nil, err
	}
	if err := d.Store.DeleteSchedule(ctx, in.ID); err != nil {
		return nil, d.scheduleStoreErr(err)
	}
	d.reloadSchedules(ctx)
	return nil, nil
}

// validateCron answers POST /schedules/validate: 200 with the next fire
// times, or 400 with the parse error.
func (d Deps) validateCron(_ context.Context, in *struct{ Body CronValidateRequest }) (*jsonOut[CronValidateResponse], error) {
	b := in.Body
	times, err := scheduler.NextFireTimes(strings.TrimSpace(b.Cron), b.Timezone, nextRunCount, time.Now())
	if err != nil {
		return nil, badRequest(err.Error())
	}
	return &jsonOut[CronValidateResponse]{CronValidateResponse{Next: formatTimes(times), OK: true}}, nil
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
func (d Deps) runSchedule(ctx context.Context, in *idIn) (*jsonOut[RunQueued], error) {
	if err := in.check(); err != nil {
		return nil, err
	}
	sc, err := d.Store.GetSchedule(ctx, in.ID)
	if e := d.scheduleStoreErr(err); e != nil {
		return nil, e
	}
	runID, err := d.Runner.Enqueue(ctx, runner.RunRequest{
		Trigger: "manual", ScheduleID: &sc.ID, TargetIDs: sc.TargetIDs})
	if err != nil {
		return nil, enqueueErr(d.Logger, err)
	}
	return &jsonOut[RunQueued]{RunQueued{RunID: runID}}, nil
}

// scheduleNext answers GET /schedules/{id}/next with the next five fire
// times of the stored expression.
func (d Deps) scheduleNext(ctx context.Context, in *idIn) (*jsonOut[ScheduleNext], error) {
	if err := in.check(); err != nil {
		return nil, err
	}
	sc, err := d.Store.GetSchedule(ctx, in.ID)
	if e := d.scheduleStoreErr(err); e != nil {
		return nil, e
	}
	if !sc.Enabled {
		return &jsonOut[ScheduleNext]{ScheduleNext{Next: []string{}}}, nil
	}
	times, err := scheduler.NextFireTimes(sc.Cron, sc.Timezone, nextRunCount, time.Now())
	if err != nil {
		return nil, badRequest(err.Error())
	}
	return &jsonOut[ScheduleNext]{ScheduleNext{Next: formatTimes(times)}}, nil
}
