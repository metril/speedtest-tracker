package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
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
func timeQuery(w http.ResponseWriter, q url.Values, name string) (string, bool) {
	raw := q.Get(name)
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
func (d Deps) resultFilterFromQuery(w http.ResponseWriter, q url.Values) (store.ResultFilter, bool) {
	f := store.ResultFilter{
		Engine: q.Get("engine"),
		Status: q.Get("status"),
		Tag:    q.Get("tag"),
	}
	from, ok := timeQuery(w, q, "from")
	if !ok {
		return store.ResultFilter{}, false
	}
	f.From = from
	to, ok := timeQuery(w, q, "to")
	if !ok {
		return store.ResultFilter{}, false
	}
	f.To = to
	if raw := q.Get("target_id"); raw != "" {
		id, ok := int64Query(w, q, "target_id", 0)
		if !ok {
			return store.ResultFilter{}, false
		}
		f.TargetID = &id
	}
	return f, true
}

// listResults answers GET /results with filters and a keyset cursor.
type listResultsIn struct {
	Limit    string `query:"limit" doc:"Maximum rows to return (default 50)."`
	Cursor   string `query:"cursor" doc:"Opaque cursor from a previous page's next_cursor."`
	TargetID string `query:"target_id" doc:"Only results of this target."`
	Engine   string `query:"engine" doc:"Only results of this engine."`
	Status   string `query:"status" doc:"Only results with this status (ok, failed, degraded)."`
	Tag      string `query:"tag" doc:"Only results carrying this tag."`
	From     string `query:"from" doc:"Earliest start time (RFC 3339 or unix seconds)."`
	To       string `query:"to" doc:"Latest start time (RFC 3339 or unix seconds)."`
}

// ResultsPage is the GET /results body.
type ResultsPage struct {
	Results    []store.Result `json:"results"`
	NextCursor string         `json:"next_cursor" doc:"Pass as cursor to fetch the next page; empty when exhausted."`
}

// SetTagsRequest is the PUT /results/{id}/tags body.
type SetTagsRequest struct {
	Tags []string `json:"tags"`
}

// ResultTags is the PUT /results/{id}/tags response.
type ResultTags struct {
	ID   int64    `json:"id"`
	Tags []string `json:"tags"`
}

type setTagsIn struct {
	ID   int64 `path:"id" minimum:"1" doc:"Resource ID."`
	Body SetTagsRequest
}

func (d Deps) listResults(ctx context.Context, in *listResultsIn) (*jsonOut[ResultsPage], error) {
	q := queryOf("limit", in.Limit, "cursor", in.Cursor, "target_id", in.TargetID, "engine", in.Engine,
		"status", in.Status, "tag", in.Tag, "from", in.From, "to", in.To)
	c := &errCapture{}
	f, ok := d.resultFilterFromQuery(c, q)
	if !ok {
		return nil, c.err()
	}
	limit, ok := intQuery(c, q, "limit", 50)
	if !ok {
		return nil, c.err()
	}
	f.Limit = limit
	cursor, ok := int64Query(c, q, "cursor", 0)
	if !ok {
		return nil, c.err()
	}
	f.Cursor = cursor

	results, next, err := d.Store.ListResults(ctx, f)
	if err != nil {
		return nil, internalErr(d.Logger, "list results failed", err)
	}
	return &jsonOut[ResultsPage]{ResultsPage{Results: results, NextCursor: cursorString(next)}}, nil
}

func (d Deps) getResult(ctx context.Context, in *idIn) (*jsonOut[*store.Result], error) {
	if err := in.check(); err != nil {
		return nil, err
	}
	res, err := d.Store.GetResult(ctx, in.ID)
	if e := storeErr(d.Logger, "result", err); e != nil {
		return nil, e
	}
	return &jsonOut[*store.Result]{res}, nil
}

func (d Deps) deleteResult(ctx context.Context, in *idIn) (*struct{}, error) {
	if err := in.check(); err != nil {
		return nil, err
	}
	if err := d.Store.DeleteResult(ctx, in.ID); err != nil {
		return nil, storeErr(d.Logger, "result", err)
	}
	return nil, nil
}

// reexecuteResult replays a stored result by re-running its target with
// the exact options_snapshot captured at result time (not the target's
// current live options); a result whose target is gone cannot be replayed.
func (d Deps) reexecuteResult(ctx context.Context, in *idIn) (*jsonOut[RunQueued], error) {
	if err := in.check(); err != nil {
		return nil, err
	}
	res, err := d.Store.GetResult(ctx, in.ID)
	if e := storeErr(d.Logger, "result", err); e != nil {
		return nil, e
	}
	if res.TargetID == nil {
		return nil, badRequest("result has no target to re-execute")
	}
	if _, err := d.Store.GetTarget(ctx, *res.TargetID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, badRequest("target of this result no longer exists")
		}
		return nil, internalErr(d.Logger, "get target failed", err)
	}
	runID, err := d.Runner.Enqueue(ctx, runner.RunRequest{
		Trigger:   "reexec",
		TargetIDs: []int64{*res.TargetID},
		Snapshots: map[int64]json.RawMessage{*res.TargetID: res.OptionsSnapshot},
	})
	if err != nil {
		return nil, enqueueErr(d.Logger, err)
	}
	return &jsonOut[RunQueued]{RunQueued{RunID: runID}}, nil
}

// setResultTags answers PUT /results/{id}/tags {"tags":[...]}.
func (d Deps) setResultTags(ctx context.Context, in *setTagsIn) (*jsonOut[ResultTags], error) {
	if err := (idIn{in.ID}).check(); err != nil {
		return nil, err
	}
	c := &errCapture{}
	if !validTagList(c, in.Body.Tags) {
		return nil, c.err()
	}
	tags, err := d.Store.SetResultTags(ctx, in.ID, in.Body.Tags)
	if e := storeErr(d.Logger, "result", err); e != nil {
		return nil, e
	}
	return &jsonOut[ResultTags]{ResultTags{ID: in.ID, Tags: tags}}, nil
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
