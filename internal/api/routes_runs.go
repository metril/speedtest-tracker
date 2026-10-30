package api

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

func (d Deps) registerRuns(api huma.API) {
	const t = "runs"
	register(api, route{Method: http.MethodGet, Path: "/api/v1/runs", ID: "listRuns", Tag: t, Summary: "List runs (newest first)"}, d.listRuns)
	register(api, route{Method: http.MethodPost, Path: "/api/v1/runs", ID: "createRun", Tag: t, Summary: "Run the given targets now", Status: 202, Errors: []int{503}}, d.createRun)
	register(api, route{Method: http.MethodGet, Path: "/api/v1/runs/{id}", ID: "getRun", Tag: t, Summary: "Get a run"}, d.getRun)
	register(api, route{Method: http.MethodDelete, Path: "/api/v1/runs/{id}", ID: "cancelRun", Tag: t, Summary: "Cancel an in-flight run", Status: 202, Errors: []int{409}}, d.cancelRun)
}
