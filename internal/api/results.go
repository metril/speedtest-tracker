package api

import (
	"net/http"

	"github.com/metril/speedtest-tracker/internal/runner"
	"github.com/metril/speedtest-tracker/internal/store"
)

// listResults answers GET /results with filters and a keyset cursor.
func (d Deps) listResults(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.ResultFilter{
		Engine: q.Get("engine"),
		Status: q.Get("status"),
		From:   q.Get("from"),
		To:     q.Get("to"),
	}
	if raw := q.Get("target_id"); raw != "" {
		id, ok := int64Query(w, r, "target_id", 0)
		if !ok {
			return
		}
		f.TargetID = &id
	}
	limit, ok := intQuery(w, r, "limit", 50)
	if !ok {
		return
	}
	f.Limit = limit
	cursor, ok := int64Query(w, r, "cursor", 0)
	if !ok {
		return
	}
	f.Cursor = cursor

	results, next, err := d.Store.ListResults(r.Context(), f)
	if err != nil {
		internalError(w, d.Logger, "list results failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"results": results, "next_cursor": cursorString(next)})
}

func (d Deps) getResult(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	res, err := d.Store.GetResult(r.Context(), id)
	if storeError(w, d.Logger, "result", err) {
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (d Deps) deleteResult(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := d.Store.DeleteResult(r.Context(), id); err != nil {
		storeError(w, d.Logger, "result", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// reexecuteResult replays a stored result. The runner's RunRequest cannot
// yet carry an options snapshot, so this re-runs the result's target with
// its current (live) options rather than the snapshot captured at result
// time; a result whose target is gone cannot be replayed.
func (d Deps) reexecuteResult(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	res, err := d.Store.GetResult(r.Context(), id)
	if storeError(w, d.Logger, "result", err) {
		return
	}
	if res.TargetID == nil {
		errBadRequest(w, "result has no target to re-execute")
		return
	}
	if _, err := d.Store.GetTarget(r.Context(), *res.TargetID); err != nil {
		errBadRequest(w, "target of this result no longer exists")
		return
	}
	runID, err := d.Runner.Enqueue(r.Context(), runner.RunRequest{
		Trigger: "reexec", TargetIDs: []int64{*res.TargetID}})
	if err != nil {
		enqueueError(w, d.Logger, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]int64{"run_id": runID})
}

// setResultTags answers PUT /results/{id}/tags {"tags":[...]}.
func (d Deps) setResultTags(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Tags []string `json:"tags"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	tags, err := d.Store.SetResultTags(r.Context(), id, body.Tags)
	if storeError(w, d.Logger, "result", err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "tags": tags})
}

func (d Deps) listTags(w http.ResponseWriter, r *http.Request) {
	tags, err := d.Store.ListTags(r.Context())
	if err != nil {
		internalError(w, d.Logger, "list tags failed", err)
		return
	}
	writeJSON(w, http.StatusOK, tags)
}
