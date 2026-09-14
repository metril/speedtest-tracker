package api

import (
	"net/http"

	"github.com/metril/speedtest-tracker/internal/auth"
)

// meResponse is what the SPA uses to decide whether to show the open-mode
// warning banner and whether to offer token management.
type meResponse struct {
	Mode    string   `json:"mode"`
	User    string   `json:"user"`
	Groups  []string `json:"groups"`
	IsAdmin bool     `json:"is_admin"`
}

// me reports the caller's resolved identity. A zero Identity in the
// context means no auth middleware is mounted (Deps.Auth is nil), which
// is treated as open mode with an admin identity.
func (d Deps) me(w http.ResponseWriter, r *http.Request) {
	id := auth.FromContext(r.Context())
	mode, isAdmin := id.Mode, id.IsAdmin
	if mode == "" {
		mode, isAdmin = "open", true
	}
	groups := id.Groups
	if groups == nil {
		groups = []string{}
	}
	writeJSON(w, http.StatusOK, meResponse{
		Mode:    mode,
		User:    id.User,
		Groups:  groups,
		IsAdmin: isAdmin,
	})
}
