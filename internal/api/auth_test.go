package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/metril/speedtest-tracker/internal/auth"
	"github.com/metril/speedtest-tracker/internal/settings"
	"github.com/metril/speedtest-tracker/internal/store"
)

// storeTokenLookup adapts *store.Store's API-token queries to
// auth.TokenLookup, whose method names differ from the store's.
type storeTokenLookup struct{ db *store.Store }

func (s storeTokenLookup) LookupToken(ctx context.Context, hash string) (int64, bool, error) {
	t, ok, err := s.db.APITokenByHash(ctx, hash)
	return t.ID, ok, err
}

func (s storeTokenLookup) TouchToken(ctx context.Context, id int64) error {
	return s.db.TouchAPIToken(ctx, id)
}

// authAdapter implements Authenticator over *auth.Middleware, reporting
// the mode it was configured with.
type authAdapter struct {
	mw   *auth.Middleware
	mode string
}

func (a authAdapter) Handler(next http.Handler) http.Handler { return a.mw.Handler(next) }
func (a authAdapter) Mode() string                           { return a.mode }

// newAPI is newTestAPI trimmed to just the handler, for tests that only
// care about routing (Deps.Auth is left nil, so every route is open).
func newAPI(t *testing.T) http.Handler {
	t.Helper()
	h, _, _ := newTestAPI(t)
	return h
}

// newAuthedAPI builds a test API with Deps.Auth configured from cfg. When
// cfg needs a bearer credential (token mode, or forward_auth with
// AllowTokens) it also issues one API token and returns its plaintext.
func newAuthedAPI(t *testing.T, cfg settings.Auth) (http.Handler, string) {
	t.Helper()
	var plain string
	h, _, _ := newTestAPIWith(t, func(d *Deps) {
		mw := auth.New(nil, storeTokenLookup{db: d.Store}, time.Now)
		if err := mw.Configure(cfg); err != nil {
			t.Fatal(err)
		}
		d.Auth = authAdapter{mw: mw, mode: cfg.Mode}

		if cfg.Mode == settings.AuthModeToken || (cfg.Mode == settings.AuthModeForward && cfg.AllowTokens) {
			p, hash, prefix, err := auth.GenerateToken()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := d.Store.CreateAPIToken(context.Background(), "test", hash, prefix); err != nil {
				t.Fatal(err)
			}
			plain = p
		}
	})
	return h, plain
}

func TestAuthProtectsAPIButNotOpsEndpoints(t *testing.T) {
	h, plain := newAuthedAPI(t, settings.Auth{Mode: settings.AuthModeToken})

	for _, path := range []string{"/healthz", "/metrics"} {
		rec := do(t, h, http.MethodGet, path, nil)
		if rec.Code == http.StatusUnauthorized {
			t.Errorf("%s = 401; ops endpoints must stay reachable for probes and scrapers", path)
		}
	}
	for _, path := range []string{"/api/v1/targets", "/api/v1/results.csv", "/api/v1/settings", "/api/v1/events"} {
		if rec := do(t, h, http.MethodGet, path, nil); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s = %d, want 401", path, rec.Code)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/targets", nil)
	req.Header.Set("Authorization", "Bearer "+plain)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("authorized request = %d", rec.Code)
	}
}

func TestEventsAcceptsTokenQueryParam(t *testing.T) {
	h, plain := newAuthedAPI(t, settings.Auth{Mode: settings.AuthModeToken})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?token="+plain, nil)
	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	go func() { time.Sleep(20 * time.Millisecond); cancel() }()
	h.ServeHTTP(rec, req)
	if rec.Code == http.StatusUnauthorized {
		t.Fatal("EventSource cannot set an Authorization header; ?token= must work on /api/v1/events")
	}
}

func TestMeReportsIdentity(t *testing.T) {
	h, _ := newAuthedAPI(t, settings.Auth{Mode: settings.AuthModeOpen})
	var body struct {
		Mode    string   `json:"mode"`
		User    string   `json:"user"`
		Groups  []string `json:"groups"`
		IsAdmin bool     `json:"is_admin"`
	}
	doJSON(t, h, http.MethodGet, "/api/v1/me", nil, &body)
	if body.Mode != "open" || !body.IsAdmin || body.Groups == nil {
		t.Fatalf("me = %+v, want open/admin with a non-nil groups array", body)
	}

	h2, _ := newAuthedAPI(t, settings.Auth{Mode: settings.AuthModeForward,
		UserHeader: "Remote-User", GroupsHeader: "Remote-Groups", GroupsSeparator: ",",
		AdminGroup: "admins", TrustedProxies: []string{"192.0.2.0/24"}})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.RemoteAddr = "192.0.2.1:1"
	req.Header.Set("Remote-User", "alice")
	req.Header.Set("Remote-Groups", "users")
	rec := httptest.NewRecorder()
	h2.ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), `"user":"alice"`) || strings.Contains(rec.Body.String(), `"is_admin":true`) {
		t.Fatalf("me = %s, want alice and is_admin false", rec.Body)
	}
}

// TestWriteGuardRejectsNonAdminMutations is a regression test: previously
// only the auth settings, tokens and iperf3 endpoints checked IsAdmin
// individually, so a forward_auth viewer identity outside the configured
// admin group could still mutate targets/results/etc. via any other
// /api/v1 route.
func TestWriteGuardRejectsNonAdminMutations(t *testing.T) {
	h, _ := newAuthedAPI(t, settings.Auth{Mode: settings.AuthModeForward,
		UserHeader: "Remote-User", GroupsHeader: "Remote-Groups", GroupsSeparator: ",",
		AdminGroup: "admins", TrustedProxies: []string{"192.0.2.0/24"}})

	viewer := func(req *http.Request) {
		req.RemoteAddr = "192.0.2.1:1"
		req.Header.Set("Remote-User", "alice")
		req.Header.Set("Remote-Groups", "users")
	}
	admin := func(req *http.Request) {
		req.RemoteAddr = "192.0.2.1:1"
		req.Header.Set("Remote-User", "bob")
		req.Header.Set("Remote-Groups", "admins")
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/targets", nil)
	viewer(getReq)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, getReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("non-admin GET /targets = %d, want 200", rec.Code)
	}

	postReq := httptest.NewRequest(http.MethodPost, "/api/v1/targets", strings.NewReader(`{}`))
	postReq.Header.Set("Content-Type", "application/json")
	viewer(postReq)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, postReq)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin POST /targets = %d %s, want 403", rec.Code, rec.Body)
	}

	adminReq := httptest.NewRequest(http.MethodPost, "/api/v1/targets", strings.NewReader(`{}`))
	adminReq.Header.Set("Content-Type", "application/json")
	admin(adminReq)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, adminReq)
	if rec.Code == http.StatusForbidden {
		t.Fatalf("admin POST /targets = %d %s, want it to reach the handler (not 403)", rec.Code, rec.Body)
	}
}

func TestNoAuthDepsMeansOpen(t *testing.T) {
	h := newAPI(t)
	if rec := do(t, h, http.MethodGet, "/api/v1/me", nil); rec.Code != http.StatusOK ||
		!strings.Contains(rec.Body.String(), `"mode":"open"`) {
		t.Fatalf("nil Deps.Auth = %d %s, want an open identity", rec.Code, rec.Body)
	}
}
