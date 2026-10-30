package api

import (
	"net/http"

	"github.com/metril/speedtest-tracker/internal/store"
)

// RunsPage is the GET /runs body.
type RunsPage struct {
	Runs       []store.Run `json:"runs"`
	NextCursor string      `json:"next_cursor" doc:"Pass as cursor to fetch the next page; empty when exhausted."`
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

func init() { registerSpec(runSpecs) }

func runSpecs() []opSpec {
	const t = "runs"
	id := []p{idParam("Run ID")}
	return []opSpec{
		{Method: http.MethodGet, Path: "/api/v1/runs", ID: "listRuns", Tag: t, Summary: "List runs (newest first)",
			Params: []p{limitParam, cursorParam, {Name: "schedule_id", Query: true, Doc: "Only runs triggered by this schedule."}}, Out: RunsPage{}},
		{Method: http.MethodPost, Path: "/api/v1/runs", ID: "createRun", Tag: t, Summary: "Run the given targets now", Body: CreateRunRequest{}, Status: 202, Out: RunQueued{}, Errors: []int{503}},
		{Method: http.MethodGet, Path: "/api/v1/runs/{id}", ID: "getRun", Tag: t, Summary: "Get a run", Params: id, Out: store.Run{}},
		{Method: http.MethodDelete, Path: "/api/v1/runs/{id}", ID: "cancelRun", Tag: t, Summary: "Cancel an in-flight run", Params: id, Status: 202, Out: RunCanceling{}, Errors: []int{409}},
	}
}
