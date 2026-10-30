package api

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

func (d Deps) registerTags(api huma.API) {
	const t = "tags"
	register(api, route{Method: http.MethodGet, Path: "/api/v1/tags", ID: "listTags", Tag: t, Summary: "List tags"}, d.listTags)
	register(api, route{Method: http.MethodPut, Path: "/api/v1/tags/{id}", ID: "renameTag", Tag: t, Summary: "Rename a tag", Errors: []int{409}}, d.renameTag)
	register(api, route{Method: http.MethodDelete, Path: "/api/v1/tags/{id}", ID: "deleteTag", Tag: t, Summary: "Delete a tag", Status: 204}, d.deleteTag)
}
