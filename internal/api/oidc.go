package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/metril/speedtest-tracker/internal/auth"
	"github.com/metril/speedtest-tracker/internal/oidcauth"
	"github.com/metril/speedtest-tracker/internal/store"
	"golang.org/x/oauth2"
)

// oidcStateCookie carries the signed, self-expiring state produced by
// oidcauth.StateCodec between GET /auth/oidc/start and
// GET /auth/oidc/callback.
const oidcStateCookie = "st_oidc_state"

// SessionStore is the narrow dependency the OIDC login endpoints need
// from the session store; *store.Store satisfies it directly.
type SessionStore interface {
	CreateSession(ctx context.Context, sess store.Session) error
	DeleteSession(ctx context.Context, id string) error
}

// authModeResponse is what GET /auth/mode reports: just the configured
// auth mode, so the unauthenticated Login page knows what to render
// (an SSO button, a token/forward_auth hint, or nothing at all) instead
// of always assuming oidc.
type authModeResponse struct {
	Mode string `json:"mode"`
}

// authMode reports the server's configured auth mode. It is deliberately
// public (mounted outside the /api/v1 auth middleware, next to
// /auth/oidc/*): the whole point is that the Login page -- which by
// definition has no session yet -- can call it to decide what to render.
// A nil Deps.Auth (no auth middleware mounted) means open mode.
func (d Deps) authMode(w http.ResponseWriter, r *http.Request) {
	mode := "open"
	if d.Auth != nil {
		mode = d.Auth.Mode()
	}
	writeJSON(w, http.StatusOK, authModeResponse{Mode: mode})
}

// oidcNotConfigured answers a request that needs OIDC (Deps.OIDC() is
// nil, meaning auth mode oidc has no valid provider yet) with a redirect
// carrying an error the SPA's login page can render.
func (d Deps) oidcNotConfigured(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/login?error=oidc_not_configured", http.StatusFound)
}

// oidcStart begins the authorization-code + PKCE flow: it stashes a
// signed state (carrying the PKCE verifier and the post-login redirect
// target) in a short-lived cookie, then redirects the browser to the
// provider.
func (d Deps) oidcStart(w http.ResponseWriter, r *http.Request) {
	provider := d.OIDC()
	if provider == nil || d.StateCodec == nil {
		d.oidcNotConfigured(w, r)
		return
	}

	returnTo := sanitizeReturnTo(r.URL.Query().Get("return_to"))

	state := randomToken()
	verifier := oauth2.GenerateVerifier()
	encoded := d.StateCodec.Encode(state, verifier, returnTo)

	http.SetCookie(w, &http.Cookie{
		Name:     oidcStateCookie,
		Value:    encoded,
		Path:     "/",
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   300,
	})

	redirectURI := externalBaseURL(r, provider.Config().RedirectBaseURL) + "/auth/oidc/callback"
	http.Redirect(w, r, provider.AuthCodeURL(state, verifier, redirectURI), http.StatusFound)
}

// oidcCallback completes the flow: it validates the state, exchanges the
// code, authorises the resulting claims and, on success, establishes a
// server-side session before redirecting to the original return_to path.
func (d Deps) oidcCallback(w http.ResponseWriter, r *http.Request) {
	provider := d.OIDC()
	if provider == nil || d.StateCodec == nil {
		d.oidcNotConfigured(w, r)
		return
	}

	redirectURI := externalBaseURL(r, provider.Config().RedirectBaseURL) + "/auth/oidc/callback"

	if errCode := r.URL.Query().Get("error"); errCode != "" {
		d.logOIDCFailure(r, "provider_error", nil, provider, redirectURI)
		redirectLoginError(w, r, "provider_error")
		return
	}

	cookie, err := r.Cookie(oidcStateCookie)
	if err != nil || cookie.Value == "" {
		d.logOIDCFailure(r, "state", nil, provider, redirectURI)
		redirectLoginError(w, r, "state")
		return
	}
	clearCookie(w, r, oidcStateCookie)

	wantState, verifier, returnTo, err := d.StateCodec.Decode(cookie.Value, d.now())
	if err != nil {
		d.logOIDCFailure(r, "state", err, provider, redirectURI)
		redirectLoginError(w, r, "state")
		return
	}
	if r.URL.Query().Get("state") != wantState {
		d.logOIDCFailure(r, "state", nil, provider, redirectURI)
		redirectLoginError(w, r, "state")
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		d.logOIDCFailure(r, "state", nil, provider, redirectURI)
		redirectLoginError(w, r, "state")
		return
	}

	claims, err := provider.Exchange(r.Context(), code, verifier, redirectURI)
	if err != nil {
		d.logOIDCFailure(r, "exchange_failed", err, provider, redirectURI)
		redirectLoginError(w, r, "exchange_failed")
		return
	}

	isAdmin, err := provider.Config().Authorize(claims)
	if errors.Is(err, oidcauth.ErrForbidden) {
		d.logOIDCFailure(r, "forbidden", err, provider, redirectURI)
		redirectLoginError(w, r, "forbidden")
		return
	}
	if err != nil {
		d.logOIDCFailure(r, "authorize_failed", err, provider, redirectURI)
		redirectLoginError(w, r, "authorize_failed")
		return
	}

	if d.Sessions == nil {
		d.oidcNotConfigured(w, r)
		return
	}

	plain, err := auth.GenerateSessionID()
	if err != nil {
		d.logOIDCFailure(r, "session_failed", err, provider, redirectURI)
		redirectLoginError(w, r, "session_failed")
		return
	}
	ttl := provider.Config().SessionTTL
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	now := d.now()
	sess := store.Session{
		ID:        auth.HashSession(plain),
		Subject:   claims.Subject,
		Email:     claims.Email,
		Name:      claims.Name,
		Username:  claims.PreferredUsername,
		Groups:    claims.Groups,
		IsAdmin:   isAdmin,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}
	if err := d.Sessions.CreateSession(r.Context(), sess); err != nil {
		internalError(w, d.Logger, "create session", err)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookie,
		Value:    plain,
		Path:     "/",
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(ttl.Seconds()),
	})
	http.Redirect(w, r, returnTo, http.StatusFound)
}

// oidcLogout deletes the caller's server-side session (if any) and clears
// the session cookie. POST answers 204 for the SPA; GET redirects to the
// login page for a plain link/bookmark.
func (d Deps) oidcLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(auth.SessionCookie); err == nil && cookie.Value != "" && d.Sessions != nil {
		_ = d.Sessions.DeleteSession(r.Context(), auth.HashSession(cookie.Value))
	}
	clearCookie(w, r, auth.SessionCookie)

	if r.Method == http.MethodGet {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d Deps) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

// sanitizeReturnTo restricts an OIDC return_to value to a same-origin
// relative path, refusing anything a browser could interpret as pointing
// elsewhere. url.Parse alone is not enough: a scheme-less, host-less
// value like `/\evil.com` still parses with Path == "/\evil.com", but
// browsers normalize the backslash to a slash and treat it as
// "//evil.com" -- a protocol-relative URL to another host. So beyond
// requiring an empty scheme/host and a leading "/", any leading "//" or
// "/\" is rejected outright; anything that fails these checks falls back
// to "/".
func sanitizeReturnTo(returnTo string) string {
	u, err := url.Parse(returnTo)
	if err != nil {
		return "/"
	}
	if u.Scheme != "" || u.Host != "" {
		return "/"
	}
	if !strings.HasPrefix(u.Path, "/") {
		return "/"
	}
	if strings.HasPrefix(u.Path, "//") || strings.HasPrefix(u.Path, `/\`) {
		return "/"
	}
	return returnTo
}

// logOIDCFailure logs a swallowed OIDC callback failure before the caller
// redirects the browser to the login page with an opaque error code, so
// the underlying cause (bad state, exchange failure, forbidden claims,
// ...) is not lost.
func (d Deps) logOIDCFailure(r *http.Request, code string, err error, provider *oidcauth.Provider, redirectURI string) {
	if d.Logger == nil {
		return
	}
	args := []any{"reason", code, "issuer", provider.Config().Issuer, "redirect_uri", redirectURI}
	if err != nil {
		args = append(args, "err", err)
	}
	d.Logger.Warn("oidc login failed", args...)
}

func redirectLoginError(w http.ResponseWriter, r *http.Request, code string) {
	http.Redirect(w, r, "/login?error="+code, http.StatusFound)
}

func clearCookie(w http.ResponseWriter, r *http.Request, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// externalBaseURL returns the externally-visible origin used to build the
// OIDC redirect URI: configured, when set, else derived from the request
// (honouring reverse-proxy headers).
func externalBaseURL(r *http.Request, configured string) string {
	if configured != "" {
		return strings.TrimRight(configured, "/")
	}
	scheme := "http"
	if isHTTPS(r) {
		scheme = "https"
	}
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	return scheme + "://" + host
}

// randomToken returns a URL-safe random string suitable for use as the
// OIDC "state" parameter.
func randomToken() string {
	s, err := auth.GenerateSessionID()
	if err != nil {
		// GenerateSessionID only fails if crypto/rand is broken, which
		// makes the whole process unsafe to continue running anyway.
		panic(err)
	}
	return s
}
