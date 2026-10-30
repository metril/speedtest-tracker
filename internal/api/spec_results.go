package api

import (
	"net/http"

	"github.com/metril/speedtest-tracker/internal/store"
)

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

// resultFilterParams are the filters shared by GET /results and the CSV export.
var resultFilterParams = []p{
	{Name: "target_id", Query: true, Doc: "Only results of this target."},
	{Name: "engine", Query: true, Type: "string", Doc: "Only results of this engine."},
	{Name: "status", Query: true, Type: "string", Doc: "Only results with this status (ok, failed, degraded)."},
	{Name: "tag", Query: true, Type: "string", Doc: "Only results carrying this tag."},
	{Name: "from", Query: true, Type: "string", Doc: "Earliest start time (RFC 3339)."},
	{Name: "to", Query: true, Type: "string", Doc: "Latest start time (RFC 3339)."},
}

func init() { registerSpec(resultSpecs) }

func resultSpecs() []opSpec {
	const t = "results"
	id := []p{idParam("Result ID")}
	return []opSpec{
		{Method: http.MethodGet, Path: "/api/v1/results", ID: "listResults", Tag: t, Summary: "List results (newest first)",
			Params: append([]p{limitParam, cursorParam}, resultFilterParams...), Out: ResultsPage{}},
		{Method: http.MethodGet, Path: "/api/v1/results/{id}", ID: "getResult", Tag: t, Summary: "Get a result", Params: id, Out: store.Result{}},
		{Method: http.MethodDelete, Path: "/api/v1/results/{id}", ID: "deleteResult", Tag: t, Summary: "Delete a result", Params: id},
		{Method: http.MethodPost, Path: "/api/v1/results/{id}/reexecute", ID: "reexecuteResult", Tag: t, Summary: "Run the result's target again with its options snapshot", Params: id, Status: 202, Out: RunQueued{}, Errors: []int{503}},
		{Method: http.MethodPut, Path: "/api/v1/results/{id}/tags", ID: "setResultTags", Tag: t, Summary: "Replace a result's tags", Params: id, Body: SetTagsRequest{}, Out: ResultTags{}},
	}
}
