package api

import (
	"net/http"

	"github.com/metril/speedtest-tracker/internal/store"
)

// RenameTagRequest is the PUT /tags/{id} body.
type RenameTagRequest struct {
	Name string `json:"name"`
}

func init() { registerSpec(tagSpecs) }

func tagSpecs() []opSpec {
	const t = "tags"
	id := []p{idParam("Tag ID")}
	return []opSpec{
		{Method: http.MethodGet, Path: "/api/v1/tags", ID: "listTags", Tag: t, Summary: "List tags", Out: []store.Tag{}},
		{Method: http.MethodPut, Path: "/api/v1/tags/{id}", ID: "renameTag", Tag: t, Summary: "Rename a tag", Params: id, Body: RenameTagRequest{}, Out: store.Tag{}, Errors: []int{409}},
		{Method: http.MethodDelete, Path: "/api/v1/tags/{id}", ID: "deleteTag", Tag: t, Summary: "Delete a tag", Params: id},
	}
}
