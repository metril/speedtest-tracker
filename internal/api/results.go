package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/metril/speedtest-tracker/internal/runner"
	"github.com/metril/speedtest-tracker/internal/store"
)

// dbTimeFormat is the timestamp layout stored in started_at (matches
// nowExpr's strftime format), so from/to filters compare correctly.
const dbTimeFormat = "2006-01-02T15:04:05.000Z"

// timeQuery parses a from/to query parameter, accepting either RFC3339 (any
// fractional-second precision) or unix seconds, and converts it to the
// DB's stored timestamp format. It answers 400 on anything else.
func timeQuery(w http.ResponseWriter, r *http.Request, name string) (string, bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return "", true
	}
	if sec, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return time.Unix(sec, 0).UTC().Format(dbTimeFormat), true
	}
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		errBadRequest(w, name+" must be RFC3339 or unix seconds")
		return "", false
	}
	return t.UTC().Format(dbTimeFormat), true
}

// resultFilterFromQuery parses the filter parameters shared by
// GET /results and GET /results.csv (engine/status/tag/from/to/target_id).
// Limit/Cursor are left zero; listResults fills them in on top.
func (d Deps) resultFilterFromQuery(w http.ResponseWriter, r *http.Request) (store.ResultFilter, bool) {
	q := r.URL.Query()
	f := store.ResultFilter{
		Engine: q.Get("engine"),
		Status: q.Get("status"),
		Tag:    q.Get("tag"),
	}
	from, ok := timeQuery(w, r, "from")
	if !ok {
		return store.ResultFilter{}, false
	}
	f.From = from
	to, ok := timeQuery(w, r, "to")
	if !ok {
		return store.ResultFilter{}, false
	}
	f.To = to
	if raw := q.Get("target_id"); raw != "" {
		id, ok := int64Query(w, r, "target_id", 0)
		if !ok {
			return store.ResultFilter{}, false
		}
		f.TargetID = &id
	}
	return f, true
}

// listResults answers GET /results with filters and a keyset cursor.
func (d Deps) listResults(w http.ResponseWriter, r *http.Request) {
	f, ok := d.resultFilterFromQuery(w, r)
	if !ok {
		return
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

// reexecuteResult replays a stored result by re-running its target with
// the exact options_snapshot captured at result time (not the target's
// current live options); a result whose target is gone cannot be replayed.
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
		if errors.Is(err, store.ErrNotFound) {
			errBadRequest(w, "target of this result no longer exists")
		} else {
			internalError(w, d.Logger, "get target failed", err)
		}
		return
	}
	runID, err := d.Runner.Enqueue(r.Context(), runner.RunRequest{
		Trigger:   "reexec",
		TargetIDs: []int64{*res.TargetID},
		Snapshots: map[int64]json.RawMessage{*res.TargetID: res.OptionsSnapshot},
	})
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
	if !validTagList(w, body.Tags) {
		return
	}
	tags, err := d.Store.SetResultTags(r.Context(), id, body.Tags)
	if storeError(w, d.Logger, "result", err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "tags": tags})
}

// validTagList enforces at most 20 distinct tags, each 1-40 characters
// (runes, not bytes, so multi-byte tags aren't unfairly truncated) after
// normalisation (trim + lowercase, matching the store's own
// normalisation). It answers 400 and reports false on a violation.
func validTagList(w http.ResponseWriter, tags []string) bool {
	seen := map[string]bool{}
	distinct := 0
	for _, t := range tags {
		norm := strings.ToLower(strings.TrimSpace(t))
		n := utf8.RuneCountInString(norm)
		if n < 1 || n > 40 {
			errBadRequest(w, "each tag must be 1-40 characters after trimming")
			return false
		}
		if !seen[norm] {
			seen[norm] = true
			distinct++
		}
	}
	if distinct > 20 {
		errBadRequest(w, "at most 20 tags allowed")
		return false
	}
	return true
}

func (d Deps) listTags(w http.ResponseWriter, r *http.Request) {
	tags, err := d.Store.ListTags(r.Context())
	if err != nil {
		internalError(w, d.Logger, "list tags failed", err)
		return
	}
	writeJSON(w, http.StatusOK, tags)
}
