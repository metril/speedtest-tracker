package api

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

func (d Deps) registerResults(api huma.API) {
	const t = "results"
	register(api, route{Method: http.MethodGet, Path: "/api/v1/results", ID: "listResults", Tag: t, Summary: "List results (newest first)"}, d.listResults)
	register(api, route{Method: http.MethodGet, Path: "/api/v1/results/{id}", ID: "getResult", Tag: t, Summary: "Get a result"}, d.getResult)
	register(api, route{Method: http.MethodDelete, Path: "/api/v1/results/{id}", ID: "deleteResult", Tag: t, Summary: "Delete a result", Status: 204}, d.deleteResult)
	register(api, route{Method: http.MethodPost, Path: "/api/v1/results/{id}/reexecute", ID: "reexecuteResult", Tag: t, Summary: "Run the result's target again with its options snapshot", Status: 202, Errors: []int{503}}, d.reexecuteResult)
	register(api, route{Method: http.MethodPut, Path: "/api/v1/results/{id}/tags", ID: "setResultTags", Tag: t, Summary: "Replace a result's tags"}, d.setResultTags)
}
