package api

import (
	"net/http"

	"github.com/metril/speedtest-tracker/internal/store"
)

func init() { registerSpec(queueSpecs) }

func queueSpecs() []opSpec {
	const t = "queues"
	return []opSpec{
		{Method: http.MethodGet, Path: "/api/v1/queues", ID: "listQueues", Tag: t, Summary: "List queues", Out: []store.Queue{}},
		{Method: http.MethodPost, Path: "/api/v1/queues", ID: "createQueue", Tag: t, Summary: "Create a queue", Body: queueBody{}, Status: 201, Out: store.Queue{}, Errors: []int{409}},
		{Method: http.MethodPut, Path: "/api/v1/queues/{id}", ID: "renameQueue", Tag: t, Summary: "Rename a queue", Params: []p{idParam("Queue ID")}, Body: queueBody{}, Out: store.Queue{}, Errors: []int{409}},
		{Method: http.MethodDelete, Path: "/api/v1/queues/{id}", ID: "deleteQueue", Tag: t, Summary: "Delete a queue", Params: []p{idParam("Queue ID")}, Errors: []int{409}},
	}
}
