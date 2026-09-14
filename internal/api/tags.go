package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/metril/speedtest-tracker/internal/store"
)

// renameTag answers PUT /tags/{id} {"name":"..."}.
func (d Deps) renameTag(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var b struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &b) {
		return
	}
	if strings.TrimSpace(b.Name) == "" {
		errBadRequest(w, "name is required")
		return
	}
	tag, err := d.Store.RenameTag(r.Context(), id, b.Name)
	if errors.Is(err, store.ErrNameConflict) {
		writeError(w, http.StatusConflict, "name_conflict", "a tag with that name already exists")
		return
	}
	if storeError(w, d.Logger, "tag", err) {
		return
	}
	writeJSON(w, http.StatusOK, tag)
}

// deleteTag answers DELETE /tags/{id}; the tag disappears from every
// result that carried it.
func (d Deps) deleteTag(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := d.Store.DeleteTag(r.Context(), id); storeError(w, d.Logger, "tag", err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
