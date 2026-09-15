package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/metril/speedtest-tracker/internal/auth"
	"github.com/metril/speedtest-tracker/internal/oidcauth"
	"github.com/metril/speedtest-tracker/internal/oidcauth/oidctest"
	"github.com/metril/speedtest-tracker/internal/settings"
	"github.com/metril/speedtest-tracker/internal/store"
)

// storeSessionLookup adapts *store.Store's session queries to
// auth.SessionLookup.
type storeSessionLookup struct{ db *store.Store }

func (s storeSessionLookup) LookupSession(ctx context.Context, hash string, now time.Time) (auth.SessionInfo, bool, error) {
	sess, ok, err := s.db.LookupSession(ctx, hash, now)
	if err != nil || !ok {
		return auth.SessionInfo{}, false, err
	}
	return auth.SessionInfo{
		Subject: sess.Subject, Email: sess.Email, Name: sess.Name,
		Groups: sess.Groups, IsAdmin: sess.IsAdmin,
	}, true, nil
}

// oidcTestEnv bundles everything a test needs to drive the OIDC login
// flow against a real store and a fake IdP.
type oidcTestEnv struct {
	h      http.Handler
	db     *store.Store
	idp    *oidctest.IDP
	now    time.Time
	setNow func(time.Time)
}

func newOIDCTestAPI(t *testing.T, cfgOverride func(*oidcauth.Config)) *oidcTestEnv {
	t.Helper()
	idp := oidctest.NewIDP(t)
	codec, err := oidcauth.NewStateCodec()
	if err != nil {
		t.Fatal(err)
	}

	cfg := oidcauth.Config{
		Issuer:       idp.URL,
		ClientID:     idp.ClientID,
		ClientSecret: idp.ClientSecret,
		GroupsClaim:  "groups",
		AdminGroup:   "admins",
		SessionTTL:   time.Hour,
	}
	if cfgOverride != nil {
		cfgOverride(&cfg)
	}
	provider, err := oidcauth.New(context.Background(), cfg, idp.Client())
	if err != nil {
		t.Fatal(err)
	}

	env := &oidcTestEnv{idp: idp, now: time.Now()}
	env.setNow = func(t time.Time) { env.now = t }

	h, db, _ := newTestAPIWith(t, func(d *Deps) {
		d.OIDC = func() *oidcauth.Provider { return provider }
		d.Sessions = d.Store
		d.StateCodec = codec
		d.Now = func() time.Time { return env.now }

		mw := auth.New(nil, storeTokenLookup{db: d.Store}, func() time.Time { return env.now })
		mw.SetSessions(storeSessionLookup{db: d.Store})
		if err := mw.Configure(settings.Auth{Mode: settings.AuthModeOIDC}); err != nil {
			t.Fatal(err)
		}
		d.Auth = authAdapter{mw: mw, mode: settings.AuthModeOIDC}
	})
	env.h = h
	env.db = db
	return env
}

func TestSanitizeReturnTo(t *testing.T) {
	cases := map[string]string{
		`/\evil.com`:       "/",
		"//evil.com":       "/",
		"https://evil.com": "/",
		"/ok?x=1":          "/ok?x=1",
		"/dashboard":       "/dashboard",
		"":                 "/",
		"not-a-path":       "/",
	}
	for in, want := range cases {
		if got := sanitizeReturnTo(in); got != want {
			t.Errorf("sanitizeReturnTo(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAuthModeOpenWhenNoAuthMounted(t *testing.T) {
	h := newAPI(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/auth/mode", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"mode":"open"`) {
		t.Fatalf("/auth/mode = %d %s, want 200 mode=open", rec.Code, rec.Body)
	}
}

func TestAuthModeReportsConfiguredMode(t *testing.T) {
	h, _ := newAuthedAPI(t, settings.Auth{Mode: settings.AuthModeToken})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/auth/mode", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"mode":"token"`) {
		t.Fatalf("/auth/mode = %d %s, want 200 mode=token", rec.Code, rec.Body)
	}
	// /auth/mode must not require the bearer token: no Authorization
	// header is set on the request above, and it still answered 200.
}

func TestOIDCStartSetsCookieAndRedirects(t *testing.T) {
	env := newOIDCTestAPI(t, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/auth/oidc/start?return_to=/dashboard", nil)
	env.h.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if loc.Query().Get("code_challenge") == "" || loc.Query().Get("code_challenge_method") != "S256" {
		t.Fatalf("Location = %s, missing PKCE challenge", rec.Header().Get("Location"))
	}
	var found bool
	for _, c := range rec.Result().Cookies() {
		if c.Name == oidcStateCookie {
			found = true
			if !c.HttpOnly {
				t.Error("state cookie must be HttpOnly")
			}
		}
	}
	if !found {
		t.Fatal("no state cookie set")
	}
}

// oidcStart performs the start step and returns the state cookie and the
// state value the IdP redirect carries.
func oidcStart(t *testing.T, h http.Handler, returnTo string) (*http.Cookie, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/auth/oidc/start?return_to="+url.QueryEscape(returnTo), nil)
	h.ServeHTTP(rec, req)
	var stateCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == oidcStateCookie {
			stateCookie = c
		}
	}
	if stateCookie == nil {
		t.Fatal("no state cookie set by /auth/oidc/start")
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	return stateCookie, loc.Query().Get("state")
}

func TestOIDCCallbackHappyPathAndMe(t *testing.T) {
	env := newOIDCTestAPI(t, nil)
	stateCookie, state := oidcStart(t, env.h, "/dashboard")

	env.idp.Issue("code-1", map[string]any{
		"sub": "user-1", "email": "alice@example.com", "name": "Alice",
		"groups": []any{"admins"},
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/oidc/callback?code=code-1&state="+state, nil)
	req.AddCookie(stateCookie)
	rec := httptest.NewRecorder()
	env.h.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/dashboard" {
		t.Fatalf("callback = %d %q", rec.Code, rec.Header().Get("Location"))
	}
	var sessionCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.SessionCookie {
			sessionCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatal("no session cookie set")
	}

	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	meReq.AddCookie(sessionCookie)
	meRec := httptest.NewRecorder()
	env.h.ServeHTTP(meRec, meReq)
	if meRec.Code != http.StatusOK {
		t.Fatalf("/api/v1/me = %d: %s", meRec.Code, meRec.Body)
	}
	body := meRec.Body.String()
	if !strings.Contains(body, `"email":"alice@example.com"`) || !strings.Contains(body, `"is_admin":true`) {
		t.Fatalf("me = %s", body)
	}
}

func TestOIDCCallbackStateMismatch(t *testing.T) {
	env := newOIDCTestAPI(t, nil)
	stateCookie, _ := oidcStart(t, env.h, "/dashboard")

	req := httptest.NewRequest(http.MethodGet, "/auth/oidc/callback?code=whatever&state=wrong-state", nil)
	req.AddCookie(stateCookie)
	rec := httptest.NewRecorder()
	env.h.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/login?error=state" {
		t.Fatalf("callback = %d %q, want 302 /login?error=state", rec.Code, rec.Header().Get("Location"))
	}
}

func TestOIDCCallbackForbiddenGroup(t *testing.T) {
	env := newOIDCTestAPI(t, func(cfg *oidcauth.Config) {
		cfg.AllowedGroups = []string{"staff"}
	})
	stateCookie, state := oidcStart(t, env.h, "/dashboard")
	env.idp.Issue("code-2", map[string]any{"sub": "user-2", "groups": []any{"other"}})

	req := httptest.NewRequest(http.MethodGet, "/auth/oidc/callback?code=code-2&state="+state, nil)
	req.AddCookie(stateCookie)
	rec := httptest.NewRecorder()
	env.h.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/login?error=forbidden" {
		t.Fatalf("callback = %d %q, want 302 /login?error=forbidden", rec.Code, rec.Header().Get("Location"))
	}
}

func TestOIDCLogoutDeletesSessionAndClearsCookie(t *testing.T) {
	env := newOIDCTestAPI(t, nil)
	stateCookie, state := oidcStart(t, env.h, "/dashboard")
	env.idp.Issue("code-3", map[string]any{"sub": "user-3", "groups": []any{}})

	cbReq := httptest.NewRequest(http.MethodGet, "/auth/oidc/callback?code=code-3&state="+state, nil)
	cbReq.AddCookie(stateCookie)
	cbRec := httptest.NewRecorder()
	env.h.ServeHTTP(cbRec, cbReq)

	var sessionCookie *http.Cookie
	for _, c := range cbRec.Result().Cookies() {
		if c.Name == auth.SessionCookie {
			sessionCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatal("no session cookie set")
	}

	logoutReq := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	logoutReq.AddCookie(sessionCookie)
	logoutRec := httptest.NewRecorder()
	env.h.ServeHTTP(logoutRec, logoutReq)
	if logoutRec.Code != http.StatusNoContent {
		t.Fatalf("logout = %d", logoutRec.Code)
	}

	if _, ok, err := env.db.LookupSession(context.Background(), auth.HashSession(sessionCookie.Value), time.Now()); err != nil || ok {
		t.Fatalf("session still present after logout: ok=%v err=%v", ok, err)
	}

	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	meReq.AddCookie(sessionCookie)
	meRec := httptest.NewRecorder()
	env.h.ServeHTTP(meRec, meReq)
	if meRec.Code != http.StatusUnauthorized {
		t.Fatalf("/api/v1/me after logout = %d, want 401", meRec.Code)
	}
}

func TestOIDCExpiredSessionIsUnauthorized(t *testing.T) {
	env := newOIDCTestAPI(t, nil)
	stateCookie, state := oidcStart(t, env.h, "/dashboard")
	env.idp.Issue("code-4", map[string]any{"sub": "user-4", "groups": []any{}})

	cbReq := httptest.NewRequest(http.MethodGet, "/auth/oidc/callback?code=code-4&state="+state, nil)
	cbReq.AddCookie(stateCookie)
	cbRec := httptest.NewRecorder()
	env.h.ServeHTTP(cbRec, cbReq)

	var sessionCookie *http.Cookie
	for _, c := range cbRec.Result().Cookies() {
		if c.Name == auth.SessionCookie {
			sessionCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatal("no session cookie set")
	}

	env.setNow(env.now.Add(2 * time.Hour)) // past the 1h SessionTTL

	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	meReq.AddCookie(sessionCookie)
	meRec := httptest.NewRecorder()
	env.h.ServeHTTP(meRec, meReq)
	if meRec.Code != http.StatusUnauthorized {
		t.Fatalf("/api/v1/me after expiry = %d, want 401", meRec.Code)
	}
}
