package api

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/metril/speedtest-tracker/internal/runner"
	"github.com/metril/speedtest-tracker/internal/store"
)

// listRuns answers GET /runs?limit&cursor.
type listRunsIn struct {
	Limit      string `query:"limit" doc:"Maximum rows to return (default 50)."`
	Cursor     string `query:"cursor" doc:"Opaque cursor from a previous page's next_cursor."`
	ScheduleID string `query:"schedule_id" doc:"Only runs triggered by this schedule."`
}

// RunsPage is the GET /runs body.
type RunsPage struct {
	NextCursor string      `json:"next_cursor" doc:"Pass as cursor to fetch the next page; empty when exhausted."`
	Runs       []store.Run `json:"runs"`
}

// CreateRunRequest is the POST /runs body.
type CreateRunRequest struct {
	TargetIDs []int64 `json:"target_ids"`
}

// RunCanceling is the 202 body returned when cancelation was requested.
type RunCanceling struct {
	RunID  int64  `json:"run_id"`
	Status string `json:"status" enum:"canceling"`
}

func (d Deps) listRuns(ctx context.Context, in *listRunsIn) (*jsonOut[RunsPage], error) {
	q := queryOf("limit", in.Limit, "cursor", in.Cursor, "schedule_id", in.ScheduleID)
	c := &errCapture{}
	limit, ok := intQuery(c, q, "limit", 50)
	if !ok {
		return nil, c.err()
	}
	cursor, ok := int64Query(c, q, "cursor", 0)
	if !ok {
		return nil, c.err()
	}
	f := store.RunFilter{Limit: limit, Cursor: cursor}
	if q.Get("schedule_id") != "" {
		sid, ok := int64Query(c, q, "schedule_id", 0)
		if !ok {
			return nil, c.err()
		}
		if sid == 0 {
			return nil, badRequest("schedule_id must be a positive integer")
		}
		f.ScheduleID = &sid
	}
	runs, next, err := d.Store.ListRuns(ctx, f)
	if err != nil {
		return nil, internalErr(d.Logger, "list runs failed", err)
	}
	return &jsonOut[RunsPage]{RunsPage{NextCursor: cursorString(next), Runs: runs}}, nil
}

func (d Deps) getRun(ctx context.Context, in *idIn) (*jsonOut[*store.Run], error) {
	if err := in.check(); err != nil {
		return nil, err
	}
	run, err := d.Store.GetRun(ctx, in.ID)
	if e := storeErr(d.Logger, "run", err); e != nil {
		return nil, e
	}
	return &jsonOut[*store.Run]{run}, nil
}

// createRun answers POST /runs {"target_ids":[...]}.
func (d Deps) createRun(ctx context.Context, in *struct{ Body CreateRunRequest }) (*jsonOut[RunQueued], error) {
	if len(in.Body.TargetIDs) == 0 {
		return nil, badRequest("target_ids must contain at least one target")
	}
	runID, err := d.Runner.Enqueue(ctx, runner.RunRequest{
		Trigger: "manual", TargetIDs: in.Body.TargetIDs})
	if err != nil {
		return nil, enqueueErr(d.Logger, err)
	}
	return &jsonOut[RunQueued]{RunQueued{RunID: runID}}, nil
}

// cancelRun answers DELETE /runs/{id}. A run the runner no longer tracks is
// already finished, which is a conflict rather than a not-found.
func (d Deps) cancelRun(ctx context.Context, in *idIn) (*jsonOut[RunCanceling], error) {
	if err := in.check(); err != nil {
		return nil, err
	}
	if _, err := d.Store.GetRun(ctx, in.ID); err != nil {
		return nil, storeErr(d.Logger, "run", err)
	}
	if !d.Runner.Cancel(in.ID) {
		return nil, conflict("not_running", "run is not in flight")
	}
	return &jsonOut[RunCanceling]{RunCanceling{RunID: in.ID, Status: "canceling"}}, nil
}

// intQuery parses an integer query parameter, answering 400 on garbage.
func intQuery(w http.ResponseWriter, q url.Values, name string, def int) (int, bool) {
	raw := q.Get(name)
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
func int64Query(w http.ResponseWriter, q url.Values, name string, def int64) (int64, bool) {
	raw := q.Get(name)
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
