package api

import (
	"net/http"
	"strings"

	"github.com/metril/speedtest-tracker/internal/auth"
	"github.com/metril/speedtest-tracker/internal/settings"
)

// maxTokenNameLen bounds the name of a created API token.
const maxTokenNameLen = 100

// listTokens returns every issued API token. The plaintext token is never
// stored, so it can never appear here — only at creation time.
func (d Deps) listTokens(w http.ResponseWriter, r *http.Request) {
	tokens, err := d.Store.ListAPITokens(r.Context())
	if err != nil {
		internalError(w, d.Logger, "list api tokens", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tokens": tokens})
}

// createToken issues a new API token and returns its plaintext exactly
// once; only the hash and a display prefix are persisted.
func (d Deps) createToken(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		errBadRequest(w, "name is required")
		return
	}
	if len(name) > maxTokenNameLen {
		errBadRequest(w, "name must be at most 100 characters")
		return
	}

	plain, hash, prefix, err := auth.GenerateToken()
	if err != nil {
		internalError(w, d.Logger, "generate api token", err)
		return
	}
	created, err := d.Store.CreateAPIToken(r.Context(), name, hash, prefix)
	if err != nil {
		internalError(w, d.Logger, "create api token", err)
		return
	}
	d.Logger.Info("api token created", "name", created.Name, "prefix", created.Prefix)
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":         created.ID,
		"name":       created.Name,
		"prefix":     created.Prefix,
		"created_at": created.CreatedAt,
		"token":      plain,
	})
}

// deleteToken revokes one API token, refusing to revoke the last token
// while auth mode is token — that would lock every caller out with no way
// back in short of an env override.
func (d Deps) deleteToken(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()

	a, err := d.Settings.Auth(ctx)
	if err != nil {
		internalError(w, d.Logger, "load auth settings", err)
		return
	}
	if a.Mode == settings.AuthModeToken || (a.Mode == settings.AuthModeForward && a.AllowTokens) {
		n, err := d.Store.CountAPITokens(ctx)
		if err != nil {
			internalError(w, d.Logger, "count api tokens", err)
			return
		}
		if n <= 1 {
			errBadRequest(w, "cannot revoke the last API token while auth mode is token")
			return
		}
	}

	if err := d.Store.DeleteAPIToken(ctx, id); err != nil {
		storeError(w, d.Logger, "api token", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
