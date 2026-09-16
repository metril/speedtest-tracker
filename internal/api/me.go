package api

import (
	"net/http"

	"github.com/metril/speedtest-tracker/internal/auth"
	"github.com/metril/speedtest-tracker/internal/settings"
)

// meResponse is what the SPA uses to decide whether to show the open-mode
// warning banner and whether to offer token management.
type meResponse struct {
	Mode        string   `json:"mode"`
	User        string   `json:"user"`
	Email       string   `json:"email,omitempty"`
	Name        string   `json:"name,omitempty"`
	Username    string   `json:"username,omitempty"`
	DisplayName string   `json:"display_name"`
	Groups      []string `json:"groups"`
	IsAdmin     bool     `json:"is_admin"`
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
		Mode:        mode,
		User:        id.User,
		Email:       id.Email,
		Name:        id.Name,
		Username:    id.Username,
		DisplayName: d.displayName(r, id, mode),
		Groups:      groups,
		IsAdmin:     isAdmin,
	})
}

// displayName resolves the name /auth/me reports for id. In non-oidc
// modes it is always id.User. In oidc mode it honours the configured
// auth.oidc_display_claim (name, preferred_username or email), falling
// back through name -> username -> email -> user for whichever claims
// the provider did not send.
func (d Deps) displayName(r *http.Request, id auth.Identity, mode string) string {
	if mode != settings.AuthModeOIDC {
		return id.User
	}

	claim := settings.OIDCDisplayClaimName
	if d.Settings != nil {
		if a, err := d.Settings.Auth(r.Context()); err == nil && a.OIDCDisplayClaim != "" {
			claim = a.OIDCDisplayClaim
		}
	}

	switch claim {
	case settings.OIDCDisplayClaimUsername:
		if id.Username != "" {
			return id.Username
		}
	case settings.OIDCDisplayClaimEmail:
		if id.Email != "" {
			return id.Email
		}
	default:
		if id.Name != "" {
			return id.Name
		}
	}

	switch {
	case id.Name != "":
		return id.Name
	case id.Username != "":
		return id.Username
	case id.Email != "":
		return id.Email
	default:
		return id.User
	}
}
