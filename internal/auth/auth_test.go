package auth_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/metril/speedtest-tracker/internal/auth"
	"github.com/metril/speedtest-tracker/internal/settings"
)

type fakeTokens struct {
	byHash map[string]int64
	err    error

	mu        sync.Mutex
	touched   []int64
	touchedCh chan int64 // optional: if non-nil, TouchToken also sends id here
}

func (f *fakeTokens) LookupToken(_ context.Context, hash string) (int64, bool, error) {
	if f.err != nil {
		return 0, false, f.err
	}
	id, ok := f.byHash[hash]
	return id, ok, nil
}

// TouchToken runs on Middleware's background worker goroutine, so access to
// touched must be synchronized.
func (f *fakeTokens) TouchToken(_ context.Context, id int64) error {
	f.mu.Lock()
	f.touched = append(f.touched, id)
	f.mu.Unlock()
	if f.touchedCh != nil {
		f.touchedCh <- id
	}
	return nil
}

func (f *fakeTokens) touchCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.touched)
}

// waitForTouch blocks until the worker reports one touch on ch, or fails
// the test after a generous timeout.
func waitForTouch(t *testing.T, ch <-chan int64) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the async touch worker")
	}
}

// blockingTokens is a TokenLookup whose TouchToken blocks until release is
// closed, used to fill the touch queue and prove a full queue drops rather
// than blocking the caller.
type blockingTokens struct {
	mu      sync.Mutex
	byHash  map[string]int64
	release chan struct{}
}

func (b *blockingTokens) LookupToken(_ context.Context, hash string) (int64, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	id, ok := b.byHash[hash]
	return id, ok, nil
}

func (b *blockingTokens) TouchToken(_ context.Context, _ int64) error {
	<-b.release
	return nil
}

func newMiddleware(t *testing.T, tk auth.TokenLookup, cfg settings.Auth) *auth.Middleware {
	t.Helper()
	m := auth.New(slog.Default(), tk, time.Now)
	if err := m.Configure(cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	return m
}

func TestOpenModeAllowsEveryone(t *testing.T) {
	m := newMiddleware(t, &fakeTokens{}, settings.Auth{Mode: settings.AuthModeOpen})
	id, err := m.Identify(httptest.NewRequest(http.MethodGet, "/api/v1/targets", nil))
	if err != nil || id.Mode != settings.AuthModeOpen || !id.IsAdmin {
		t.Fatalf("open mode = %+v, %v; want an anonymous admin identity", id, err)
	}
}

func TestForwardAuthRequiresTrustedProxy(t *testing.T) {
	cfg := settings.Auth{Mode: settings.AuthModeForward, UserHeader: "Remote-User",
		GroupsHeader: "Remote-Groups", GroupsSeparator: ",", TrustedProxies: []string{"10.0.0.0/8"}}
	m := newMiddleware(t, &fakeTokens{}, cfg)

	trusted := httptest.NewRequest(http.MethodGet, "/api/v1/targets", nil)
	trusted.RemoteAddr = "10.1.2.3:5555"
	trusted.Header.Set("Remote-User", "alice")
	trusted.Header.Set("Remote-Groups", "users,admins")
	id, err := m.Identify(trusted)
	if err != nil || id.User != "alice" || len(id.Groups) != 2 {
		t.Fatalf("trusted request = %+v, %v", id, err)
	}

	spoofed := httptest.NewRequest(http.MethodGet, "/api/v1/targets", nil)
	spoofed.RemoteAddr = "203.0.113.9:5555"
	spoofed.Header.Set("Remote-User", "mallory")
	if _, err := m.Identify(spoofed); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized — headers from an untrusted peer must be ignored, not trusted", err)
	}
}

func TestForwardAuthWithNoTrustedProxiesDeniesEverything(t *testing.T) {
	m := newMiddleware(t, &fakeTokens{}, settings.Auth{Mode: settings.AuthModeForward,
		UserHeader: "Remote-User", TrustedProxies: nil})
	r := httptest.NewRequest(http.MethodGet, "/api/v1/targets", nil)
	r.RemoteAddr = "10.1.2.3:5555"
	r.Header.Set("Remote-User", "alice")
	if _, err := m.Identify(r); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatal("an empty trusted-proxy list must fail closed")
	}
}

func TestForwardAuthMissingUserHeaderIs401(t *testing.T) {
	m := newMiddleware(t, &fakeTokens{}, settings.Auth{Mode: settings.AuthModeForward,
		UserHeader: "Remote-User", TrustedProxies: []string{"10.0.0.0/8"}})
	r := httptest.NewRequest(http.MethodGet, "/api/v1/targets", nil)
	r.RemoteAddr = "10.1.2.3:5555"
	if _, err := m.Identify(r); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatal("a trusted proxy that forwards no user must still be unauthorized")
	}
}

func TestAdminGroupMembership(t *testing.T) {
	cfg := settings.Auth{Mode: settings.AuthModeForward, UserHeader: "Remote-User",
		GroupsHeader: "Remote-Groups", GroupsSeparator: ",", AdminGroup: "admins",
		TrustedProxies: []string{"10.0.0.0/8"}}
	m := newMiddleware(t, &fakeTokens{}, cfg)
	for groups, want := range map[string]bool{"users,admins": true, "users": false, " admins ": true} {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/targets", nil)
		r.RemoteAddr = "10.0.0.1:1"
		r.Header.Set("Remote-User", "alice")
		r.Header.Set("Remote-Groups", groups)
		id, err := m.Identify(r)
		if err != nil || id.IsAdmin != want {
			t.Errorf("groups %q -> is_admin %v (err %v), want %v", groups, id.IsAdmin, err, want)
		}
	}
	// With no admin group configured everyone who authenticates is an admin.
	cfg.AdminGroup = ""
	m2 := newMiddleware(t, &fakeTokens{}, cfg)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/targets", nil)
	r.RemoteAddr = "10.0.0.1:1"
	r.Header.Set("Remote-User", "alice")
	if id, _ := m2.Identify(r); !id.IsAdmin {
		t.Error("no admin group configured must mean every authenticated user is an admin")
	}
}

func TestTokenModeBearer(t *testing.T) {
	plain, hash, prefix, err := auth.GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(plain, "stt_") || len(prefix) == 0 || !strings.HasPrefix(plain, prefix) {
		t.Fatalf("GenerateToken = %q / %q", plain, prefix)
	}
	if hash != auth.HashToken(plain) || strings.Contains(hash, plain) {
		t.Fatal("hash must be a digest of the plaintext, not derived from it reversibly")
	}
	tk := &fakeTokens{byHash: map[string]int64{hash: 7}}
	m := newMiddleware(t, tk, settings.Auth{Mode: settings.AuthModeToken})

	r := httptest.NewRequest(http.MethodGet, "/api/v1/targets", nil)
	r.Header.Set("Authorization", "Bearer "+plain)
	id, err := m.Identify(r)
	if err != nil || id.TokenID != 7 || !id.IsAdmin {
		t.Fatalf("bearer = %+v, %v", id, err)
	}

	bad := httptest.NewRequest(http.MethodGet, "/api/v1/targets", nil)
	bad.Header.Set("Authorization", "Bearer stt_wrong")
	if _, err := m.Identify(bad); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatalf("unknown token err = %v, want ErrUnauthorized", err)
	}
	if _, err := m.Identify(httptest.NewRequest(http.MethodGet, "/api/v1/targets", nil)); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatal("no Authorization header in token mode must be unauthorized")
	}
}

func TestTokenQueryParamOnlyOnEvents(t *testing.T) {
	plain, hash, _, _ := auth.GenerateToken()
	m := newMiddleware(t, &fakeTokens{byHash: map[string]int64{hash: 7}},
		settings.Auth{Mode: settings.AuthModeToken})

	ok := httptest.NewRequest(http.MethodGet, "/api/v1/events?token="+plain, nil)
	if id, err := m.Identify(ok); err != nil || id.TokenID != 7 {
		t.Fatalf("events with ?token= = %+v, %v; EventSource cannot set headers", id, err)
	}
	no := httptest.NewRequest(http.MethodGet, "/api/v1/targets?token="+plain, nil)
	if _, err := m.Identify(no); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatal("?token= must be accepted on /api/v1/events only, so tokens stay out of general access logs")
	}
}

func TestForwardAuthAlsoAcceptsTokensWhenAllowed(t *testing.T) {
	plain, hash, _, _ := auth.GenerateToken()
	cfg := settings.Auth{Mode: settings.AuthModeForward, UserHeader: "Remote-User",
		TrustedProxies: []string{"10.0.0.0/8"}, AllowTokens: true}
	m := newMiddleware(t, &fakeTokens{byHash: map[string]int64{hash: 7}}, cfg)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/targets", nil)
	r.RemoteAddr = "203.0.113.9:1" // not a trusted proxy: the token is the credential
	r.Header.Set("Authorization", "Bearer "+plain)
	if id, err := m.Identify(r); err != nil || id.TokenID != 7 {
		t.Fatalf("token in forward_auth mode = %+v, %v", id, err)
	}
	cfg.AllowTokens = false
	m2 := newMiddleware(t, &fakeTokens{byHash: map[string]int64{hash: 7}}, cfg)
	if _, err := m2.Identify(r); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatal("tokens must be refused in forward_auth mode when allow_tokens is off")
	}
}

func TestConfigureRejectsBadCIDRAndKeepsPreviousConfig(t *testing.T) {
	m := newMiddleware(t, &fakeTokens{}, settings.Auth{Mode: settings.AuthModeOpen})
	err := m.Configure(settings.Auth{Mode: settings.AuthModeForward, TrustedProxies: []string{"not-a-cidr"}})
	if err == nil || !strings.Contains(err.Error(), "not-a-cidr") {
		t.Fatalf("Configure err = %v, want one naming the bad CIDR", err)
	}
	if id, err := m.Identify(httptest.NewRequest(http.MethodGet, "/x", nil)); err != nil || id.Mode != settings.AuthModeOpen {
		t.Fatalf("a rejected Configure must leave the previous config in place, got %+v %v", id, err)
	}
}

func TestHandlerWritesJSON401AndPopulatesContext(t *testing.T) {
	m := newMiddleware(t, &fakeTokens{}, settings.Auth{Mode: settings.AuthModeToken})
	var seen auth.Identity
	h := m.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = auth.FromContext(r.Context())
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/targets", nil))
	if rec.Code != http.StatusUnauthorized || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("status %d content-type %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if seen.Mode != "" {
		t.Fatal("the handler ran despite a 401")
	}
}

func TestTouchIsThrottled(t *testing.T) {
	plain, hash, _, _ := auth.GenerateToken()
	tk := &fakeTokens{byHash: map[string]int64{hash: 7}, touchedCh: make(chan int64, 8)}
	now := time.Now()
	m := auth.New(slog.Default(), tk, func() time.Time { return now })
	t.Cleanup(m.Close)
	m.Configure(settings.Auth{Mode: settings.AuthModeToken})
	r := httptest.NewRequest(http.MethodGet, "/api/v1/targets", nil)
	r.Header.Set("Authorization", "Bearer "+plain)
	for i := 0; i < 5; i++ {
		m.Identify(r)
	}
	waitForTouch(t, tk.touchedCh)
	if n := tk.touchCount(); n != 1 {
		t.Fatalf("touched %d times, want 1 — last_used_at must not be a write per request", n)
	}
	now = now.Add(2 * time.Minute)
	m.Identify(r)
	waitForTouch(t, tk.touchedCh)
	if n := tk.touchCount(); n != 2 {
		t.Fatalf("touched %d times after the throttle window, want 2", n)
	}
}

// TestTouchRunsOffRequestGoroutine is a regression test for review item 6:
// maybeTouch must not call TouchToken synchronously on the request
// goroutine. It uses a TouchToken that blocks until released, and asserts
// Identify still returns promptly.
func TestTouchRunsOffRequestGoroutine(t *testing.T) {
	plain, hash, _, _ := auth.GenerateToken()
	tk := &blockingTokens{byHash: map[string]int64{hash: 1}, release: make(chan struct{})}
	m := auth.New(slog.Default(), tk, time.Now)
	t.Cleanup(func() { close(tk.release); m.Close() })
	if err := m.Configure(settings.Auth{Mode: settings.AuthModeToken}); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodGet, "/api/v1/targets", nil)
	r.Header.Set("Authorization", "Bearer "+plain)

	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, err := m.Identify(r); err != nil {
			t.Error(err)
		}
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Identify blocked on TouchToken — the touch must be dispatched to the background worker")
	}
}

// TestTouchQueueDropsWhenFull is a regression test for review item 6's
// bound: once the touch queue (cap 64) is full, further touches must be
// dropped, never block the caller.
func TestTouchQueueDropsWhenFull(t *testing.T) {
	const n = 200
	tk := &blockingTokens{byHash: map[string]int64{}, release: make(chan struct{})}
	plains := make([]string, n)
	for i := 0; i < n; i++ {
		p := fmt.Sprintf("tok-%d", i)
		plains[i] = p
		tk.byHash[auth.HashToken(p)] = int64(i + 1)
	}
	m := auth.New(slog.Default(), tk, time.Now)
	t.Cleanup(func() { close(tk.release); m.Close() })
	if err := m.Configure(settings.Auth{Mode: settings.AuthModeToken}); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for _, p := range plains {
			r := httptest.NewRequest(http.MethodGet, "/api/v1/targets", nil)
			r.Header.Set("Authorization", "Bearer "+p)
			if _, err := m.Identify(r); err != nil {
				t.Error(err)
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("distinct-token touches blocked the caller once the queue filled — a full queue must drop, not block")
	}
}

// TestCloseStopsWorkerCleanly is a regression test for review item 6: Close
// must stop the background worker and be safe to call more than once.
func TestCloseStopsWorkerCleanly(t *testing.T) {
	m := auth.New(slog.Default(), &fakeTokens{}, time.Now)
	m.Close()
	m.Close()
}
