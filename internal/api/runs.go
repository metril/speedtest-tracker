package api

import (
	"net/http"
	"strconv"

	"github.com/metril/speedtest-tracker/internal/runner"
	"github.com/metril/speedtest-tracker/internal/store"
)

// listRuns answers GET /runs?limit&cursor.
func (d Deps) listRuns(w http.ResponseWriter, r *http.Request) {
	limit, ok := intQuery(w, r, "limit", 50)
	if !ok {
		return
	}
	cursor, ok := int64Query(w, r, "cursor", 0)
	if !ok {
		return
	}
	f := store.RunFilter{Limit: limit, Cursor: cursor}
	if r.URL.Query().Get("schedule_id") != "" {
		sid, ok := int64Query(w, r, "schedule_id", 0)
		if !ok {
			return
		}
		if sid == 0 {
			errBadRequest(w, "schedule_id must be a positive integer")
			return
		}
		f.ScheduleID = &sid
	}
	runs, next, err := d.Store.ListRuns(r.Context(), f)
	if err != nil {
		internalError(w, d.Logger, "list runs failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"runs": runs, "next_cursor": cursorString(next)})
}

func (d Deps) getRun(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	run, err := d.Store.GetRun(r.Context(), id)
	if storeError(w, d.Logger, "run", err) {
		return
	}
	writeJSON(w, http.StatusOK, run)
}

// createRun answers POST /runs {"target_ids":[...]}.
func (d Deps) createRun(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TargetIDs []int64 `json:"target_ids"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if len(body.TargetIDs) == 0 {
		errBadRequest(w, "target_ids must contain at least one target")
		return
	}
	runID, err := d.Runner.Enqueue(r.Context(), runner.RunRequest{
		Trigger: "manual", TargetIDs: body.TargetIDs})
	if err != nil {
		enqueueError(w, d.Logger, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]int64{"run_id": runID})
}

// cancelRun answers DELETE /runs/{id}. A run the runner no longer tracks is
// already finished, which is a conflict rather than a not-found.
func (d Deps) cancelRun(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if _, err := d.Store.GetRun(r.Context(), id); storeError(w, d.Logger, "run", err) {
		return
	}
	if !d.Runner.Cancel(id) {
		writeError(w, http.StatusConflict, "not_running", "run is not in flight")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"run_id": id, "status": "canceling"})
}

// intQuery parses an integer query parameter, answering 400 on garbage.
func intQuery(w http.ResponseWriter, r *http.Request, name string, def int) (int, bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return def, true
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 0 {
		errBadRequest(w, name+" must be a non-negative integer")
		return 0, false
	}
	return v, true
}

// int64Query parses an int64 query parameter, answering 400 on garbage.
func int64Query(w http.ResponseWriter, r *http.Request, name string, def int64) (int64, bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return def, true
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || v < 0 {
		errBadRequest(w, name+" must be a non-negative integer")
		return 0, false
	}
	return v, true
}

// cursorString renders a keyset cursor, empty when the listing is done.
func cursorString(next int64) string {
	if next == 0 {
		return ""
	}
	return strconv.FormatInt(next, 10)
}
