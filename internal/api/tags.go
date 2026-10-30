package api

import (
	"context"
	"errors"
	"strings"

	"github.com/metril/speedtest-tracker/internal/store"
)

// renameTag answers PUT /tags/{id} {"name":"..."}.
func (d Deps) renameTag(ctx context.Context, in *renameTagIn) (*jsonOut[store.Tag], error) {
	if err := (idIn{in.ID}).check(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Body.Name) == "" {
		return nil, badRequest("name is required")
	}
	tag, err := d.Store.RenameTag(ctx, in.ID, in.Body.Name)
	if errors.Is(err, store.ErrNameConflict) {
		return nil, conflict("name_conflict", "a tag with that name already exists")
	}
	if e := storeErr(d.Logger, "tag", err); e != nil {
		return nil, e
	}
	return &jsonOut[store.Tag]{tag}, nil
}

// RenameTagRequest is the PUT /tags/{id} body.
type RenameTagRequest struct {
	Name string `json:"name"`
}

type renameTagIn struct {
	ID   int64 `path:"id" minimum:"1" doc:"Resource ID."`
	Body RenameTagRequest
}

func (d Deps) listTags(ctx context.Context, _ *struct{}) (*jsonOut[[]store.Tag], error) {
	tags, err := d.Store.ListTags(ctx)
	if err != nil {
		return nil, internalErr(d.Logger, "list tags failed", err)
	}
	return &jsonOut[[]store.Tag]{tags}, nil
}

// deleteTag answers DELETE /tags/{id}; the tag disappears from every
// result that carried it.
func (d Deps) deleteTag(ctx context.Context, in *idIn) (*struct{}, error) {
	if err := in.check(); err != nil {
		return nil, err
	}
	if err := d.Store.DeleteTag(ctx, in.ID); err != nil {
		return nil, storeErr(d.Logger, "tag", err)
	}
	return nil, nil
}
