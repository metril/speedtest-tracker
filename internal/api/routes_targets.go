package api

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

func (d Deps) registerTargets(api huma.API) {
	const t = "targets"
	register(api, route{Method: http.MethodGet, Path: "/api/v1/targets", ID: "listTargets", Tag: t, Summary: "List targets"}, d.listTargets)
	register(api, route{Method: http.MethodPost, Path: "/api/v1/targets", ID: "createTarget", Tag: t, Summary: "Create a target", Status: 201}, d.createTarget)
	register(api, route{Method: http.MethodPost, Path: "/api/v1/targets/test", ID: "testTarget", Tag: t, Summary: "Validate a target's engine options without saving"}, d.testTarget)
	register(api, route{Method: http.MethodGet, Path: "/api/v1/targets/deleted", ID: "listDeletedTargets", Tag: t, Summary: "List soft-deleted targets"}, d.listDeletedTargets)
	register(api, route{Method: http.MethodPost, Path: "/api/v1/targets/deleted/{id}/restore", ID: "restoreDeletedTarget", Tag: t, Summary: "Restore a deleted target", Errors: []int{409}}, d.restoreDeletedTarget)
	register(api, route{Method: http.MethodGet, Path: "/api/v1/targets/{id}", ID: "getTarget", Tag: t, Summary: "Get a target"}, d.getTarget)
	register(api, route{Method: http.MethodPut, Path: "/api/v1/targets/{id}", ID: "updateTarget", Tag: t, Summary: "Replace a target"}, d.updateTarget)
	register(api, route{Method: http.MethodDelete, Path: "/api/v1/targets/{id}", ID: "deleteTarget", Tag: t, Summary: "Delete a target", Status: 204}, d.deleteTarget)
	register(api, route{Method: http.MethodPost, Path: "/api/v1/targets/{id}/run", ID: "runTarget", Tag: t, Summary: "Run a target now", Status: 202, Errors: []int{503}}, d.runTarget)
	register(api, route{Method: http.MethodGet, Path: "/api/v1/targets/{id}/latest", ID: "getTargetLatest", Tag: t, Summary: "Latest result of a target"}, d.targetLatest)
	register(api, route{Method: http.MethodGet, Path: "/api/v1/targets/{id}/history", ID: "getTargetHistory", Tag: t, Summary: "Bucketed history of a target"}, d.targetHistory)
	register(api, route{Method: http.MethodGet, Path: "/api/v1/targets/{id}/revisions", ID: "listTargetRevisions", Tag: t, Summary: "Revision history of a target"}, d.listTargetRevisions)
	register(api, route{Method: http.MethodPost, Path: "/api/v1/targets/{id}/revisions/{version}/revert", ID: "revertTargetRevision", Tag: t, Summary: "Revert a target to a revision"}, d.revertTargetRevision)
}
