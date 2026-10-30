package api

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

func (d Deps) registerQueues(api huma.API) {
	const t = "queues"
	register(api, route{Method: http.MethodGet, Path: "/api/v1/queues", ID: "listQueues", Tag: t, Summary: "List queues"}, d.listQueues)
	register(api, route{Method: http.MethodPost, Path: "/api/v1/queues", ID: "createQueue", Tag: t, Summary: "Create a queue", Status: 201, Errors: []int{409}}, d.createQueue)
	register(api, route{Method: http.MethodPut, Path: "/api/v1/queues/{id}", ID: "renameQueue", Tag: t, Summary: "Rename a queue", Errors: []int{409}}, d.updateQueue)
	register(api, route{Method: http.MethodDelete, Path: "/api/v1/queues/{id}", ID: "deleteQueue", Tag: t, Summary: "Delete a queue", Status: 204, Errors: []int{409}}, d.deleteQueue)
}
