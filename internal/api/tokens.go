package api

import (
	"context"
	"strings"

	"github.com/metril/speedtest-tracker/internal/auth"
	"github.com/metril/speedtest-tracker/internal/settings"
	"github.com/metril/speedtest-tracker/internal/store"
)

// maxTokenNameLen bounds the name of a created API token.
const maxTokenNameLen = 100

// TokenList is the GET /settings/tokens body.
type TokenList struct {
	Tokens []store.APIToken `json:"tokens"`
}

// CreateTokenRequest is the POST /settings/tokens body.
type CreateTokenRequest struct {
	Name string `json:"name"`
}

// CreatedToken is returned once, when a token is created: the only time
// the plaintext token is available.
type CreatedToken struct {
	CreatedAt string `json:"created_at"`
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Prefix    string `json:"prefix"`
	Token     string `json:"token" doc:"The plaintext token; shown only in this response."`
}

// listTokens returns every issued API token. The plaintext token is never
// stored, so it can never appear here — only at creation time.
func (d Deps) listTokens(ctx context.Context, _ *struct{}) (*jsonOut[TokenList], error) {
	if !requestIsAdmin(ctx) {
		return nil, forbidden("admin access required")
	}
	tokens, err := d.Store.ListAPITokens(ctx)
	if err != nil {
		return nil, internalErr(d.Logger, "list api tokens", err)
	}
	return &jsonOut[TokenList]{TokenList{Tokens: tokens}}, nil
}

// createToken issues a new API token and returns its plaintext exactly
// once; only the hash and a display prefix are persisted.
func (d Deps) createToken(ctx context.Context, in *struct{ Body CreateTokenRequest }) (*jsonOut[CreatedToken], error) {
	if !requestIsAdmin(ctx) {
		return nil, forbidden("admin access required")
	}
	// A token must not be able to mint further tokens: that would let a
	// leaked token outlive its own revocation.
	if requestAuthIsToken(ctx) {
		return nil, forbidden("creating tokens requires a forward-auth or open-mode session")
	}
	name := strings.TrimSpace(in.Body.Name)
	if name == "" {
		return nil, badRequest("name is required")
	}
	if len(name) > maxTokenNameLen {
		return nil, badRequest("name must be at most 100 characters")
	}

	plain, hash, prefix, err := auth.GenerateToken()
	if err != nil {
		return nil, internalErr(d.Logger, "generate api token", err)
	}
	created, err := d.Store.CreateAPIToken(ctx, name, hash, prefix)
	if err != nil {
		return nil, internalErr(d.Logger, "create api token", err)
	}
	d.Logger.Info("api token created", "name", created.Name, "prefix", created.Prefix)
	return &jsonOut[CreatedToken]{CreatedToken{
		CreatedAt: created.CreatedAt, ID: created.ID, Name: created.Name,
		Prefix: created.Prefix, Token: plain,
	}}, nil
}

// deleteToken revokes one API token, refusing to revoke the last token
// while auth mode is token — that would lock every caller out with no way
// back in short of an env override.
func (d Deps) deleteToken(ctx context.Context, in *idIn) (*struct{}, error) {
	if !requestIsAdmin(ctx) {
		return nil, forbidden("admin access required")
	}
	if err := in.check(); err != nil {
		return nil, err
	}
	id := in.ID

	a, err := d.Settings.Auth(ctx)
	if err != nil {
		return nil, internalErr(d.Logger, "load auth settings", err)
	}
	if a.Mode == settings.AuthModeToken || (a.Mode == settings.AuthModeForward && a.AllowTokens) {
		n, err := d.Store.CountAPITokens(ctx)
		if err != nil {
			return nil, internalErr(d.Logger, "count api tokens", err)
		}
		if n <= 1 {
			return nil, badRequest("cannot revoke the last API token while auth mode is token")
		}
	}

	if err := d.Store.DeleteAPIToken(ctx, id); err != nil {
		return nil, storeErr(d.Logger, "api token", err)
	}
	return nil, nil
}
