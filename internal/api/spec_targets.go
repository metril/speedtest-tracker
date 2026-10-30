package api

import (
	"net/http"

	"github.com/metril/speedtest-tracker/internal/store"
)

// Documentation-only mirrors of ad hoc JSON documents the handlers build
// as maps or anonymous structs; they give the OpenAPI schemas stable names.
// The handlers do not use them.

// RunQueued is the 202 body returned when a run was enqueued.
type RunQueued struct {
	RunID int64 `json:"run_id" doc:"ID of the run that was enqueued."`
}

// TargetTestResponse is returned by POST /targets/test.
type TargetTestResponse struct {
	OK     bool   `json:"ok"`
	Engine string `json:"engine"`
}

// HistoryResponse is the GET /targets/{id}/history body.
type HistoryResponse struct {
	TargetID      int64                `json:"target_id"`
	From          string               `json:"from"`
	To            string               `json:"to"`
	BucketSeconds int                  `json:"bucket_seconds"`
	Points        []store.HistoryPoint `json:"points"`
}

var (
	rangeParam  = p{Name: "range", Query: true, Type: "string", Doc: "Named window ending now.", Enum: []any{"24h", "7d", "30d"}}
	offsetParam = p{Name: "offset", Query: true, Doc: "1 shifts the window back by its own span (previous period); 0 or omitted keeps it.", Enum: []any{0, 1}}
	limitParam  = p{Name: "limit", Query: true, Doc: "Maximum rows to return."}
	cursorParam = p{Name: "cursor", Query: true, Type: "string", Doc: "Opaque cursor from a previous page's next_cursor."}
)

func init() { registerSpec(targetSpecs) }

func targetSpecs() []opSpec {
	const t = "targets"
	return []opSpec{
		{Method: http.MethodGet, Path: "/api/v1/targets", ID: "listTargets", Tag: t, Summary: "List targets", Out: []store.Target{}},
		{Method: http.MethodPost, Path: "/api/v1/targets", ID: "createTarget", Tag: t, Summary: "Create a target", Body: targetBody{}, Status: 201, Out: store.Target{}},
		{Method: http.MethodPost, Path: "/api/v1/targets/test", ID: "testTarget", Tag: t, Summary: "Validate a target's engine options without saving", Body: targetBody{}, Out: TargetTestResponse{}},
		{Method: http.MethodGet, Path: "/api/v1/targets/deleted", ID: "listDeletedTargets", Tag: t, Summary: "List soft-deleted targets", Out: []store.DeletedTarget{}},
		{Method: http.MethodPost, Path: "/api/v1/targets/deleted/{id}/restore", ID: "restoreDeletedTarget", Tag: t, Summary: "Restore a deleted target", Params: []p{idParam("Target ID")}, Out: store.Target{}, Errors: []int{409}},
		{Method: http.MethodGet, Path: "/api/v1/targets/{id}", ID: "getTarget", Tag: t, Summary: "Get a target", Params: []p{idParam("Target ID")}, Out: store.Target{}},
		{Method: http.MethodPut, Path: "/api/v1/targets/{id}", ID: "updateTarget", Tag: t, Summary: "Replace a target", Params: []p{idParam("Target ID")}, Body: targetBody{}, Out: store.Target{}},
		{Method: http.MethodDelete, Path: "/api/v1/targets/{id}", ID: "deleteTarget", Tag: t, Summary: "Delete a target", Params: []p{idParam("Target ID")}},
		{Method: http.MethodPost, Path: "/api/v1/targets/{id}/run", ID: "runTarget", Tag: t, Summary: "Run a target now", Params: []p{idParam("Target ID")}, Status: 202, Out: RunQueued{}, Errors: []int{503}},
		{Method: http.MethodGet, Path: "/api/v1/targets/{id}/latest", ID: "getTargetLatest", Tag: t, Summary: "Latest result of a target", Params: []p{idParam("Target ID")}, Out: store.Result{}},
		{Method: http.MethodGet, Path: "/api/v1/targets/{id}/history", ID: "getTargetHistory", Tag: t, Summary: "Bucketed history of a target",
			Params: []p{idParam("Target ID"), rangeParam, offsetParam,
				{Name: "from", Query: true, Type: "string", Doc: "Explicit window start (RFC 3339); use with to instead of range."},
				{Name: "to", Query: true, Type: "string", Doc: "Explicit window end (RFC 3339)."}},
			Out: HistoryResponse{}},
		{Method: http.MethodGet, Path: "/api/v1/targets/{id}/revisions", ID: "listTargetRevisions", Tag: t, Summary: "Revision history of a target", Params: []p{idParam("Target ID")}, Out: []revisionOut{}},
		{Method: http.MethodPost, Path: "/api/v1/targets/{id}/revisions/{version}/revert", ID: "revertTargetRevision", Tag: t, Summary: "Revert a target to a revision",
			Params: []p{idParam("Target ID"), {Name: "version", Doc: "Revision version", Required: true}}, Out: store.Target{}},
	}
}
