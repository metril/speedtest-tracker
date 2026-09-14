package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/metril/speedtest-tracker/internal/auth"
	"github.com/metril/speedtest-tracker/internal/settings"
	"github.com/metril/speedtest-tracker/internal/store"
)

// jsonRequest builds a JSON request without sending it, so a test can set
// fields (RemoteAddr, headers) before serving it.
func jsonRequest(t *testing.T, method, path string, body any) *http.Request {
	t.Helper()
	buf, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// withEnv seeds the settings store from kv ("KEY=value" pairs) via
// SeedFromEnv, exactly as the process does at boot.
func withEnv(kv ...string) settingsAPIOption {
	return func(d *Deps) {
		if _, err := d.Settings.SeedFromEnv(context.Background(), kv); err != nil {
			panic(err)
		}
	}
}

// newTestAPIWithSettings is newTestAPI with a settings.Store wired into
// Deps.Settings so the /api/v1/settings routes are mounted.
func newTestAPIWithSettings(t *testing.T) (http.Handler, *store.Store, *settings.Store) {
	t.Helper()
	var st *settings.Store
	h, db, _ := newTestAPIWith(t, func(d *Deps) {
		s, err := settings.New(context.Background(), d.Store)
		if err != nil {
			t.Fatal(err)
		}
		st = s
		d.Settings = s
	})
	return h, db, st
}

func TestGetSettingsMasksSecrets(t *testing.T) {
	h, _, st := newTestAPIWithSettings(t)
	st.Set(context.Background(), settings.KeyVMAuthHeader, "Bearer supersecret")
	rec := do(t, h, http.MethodGet, "/api/v1/settings", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "supersecret") {
		t.Fatalf("secret leaked: %s", rec.Body)
	}
	var body struct {
		Integrations settings.Integrations `json:"integrations"`
		General      settings.General      `json:"general"`
	}
	json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Integrations.VMAuthHeader != settings.MaskedSecret {
		t.Fatalf("vm_auth_header = %q, want %q", body.Integrations.VMAuthHeader, settings.MaskedSecret)
	}
	if body.Integrations.VLAuthHeader != "" {
		t.Fatalf("unset secret must be empty, got %q", body.Integrations.VLAuthHeader)
	}
	if body.General.RetentionDaysResults == 0 {
		t.Fatal("general section missing")
	}
}

func TestPutSettingsPreservesMaskedSecret(t *testing.T) {
	h, _, st := newTestAPIWithSettings(t)
	ctx := context.Background()
	st.Set(ctx, settings.KeyVMAuthHeader, "Bearer supersecret")
	rec := do(t, h, http.MethodPut, "/api/v1/settings", map[string]any{
		"integrations": map[string]any{
			"vm_enabled": true, "vm_url": "http://vm:8428", "vm_auth_header": settings.MaskedSecret,
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	i, _ := st.Integrations(ctx)
	if i.VMAuthHeader != "Bearer supersecret" {
		t.Fatalf("masked sentinel overwrote the stored secret: %q", i.VMAuthHeader)
	}
	if !i.VMEnabled || i.VMURL != "http://vm:8428" {
		t.Fatalf("integrations = %+v", i)
	}
}

func TestPutSettingsClearsSecretOnExplicitEmpty(t *testing.T) {
	h, _, st := newTestAPIWithSettings(t)
	ctx := context.Background()
	st.Set(ctx, settings.KeyVLAuthHeader, "old")
	do(t, h, http.MethodPut, "/api/v1/settings", map[string]any{
		"integrations": map[string]any{"vl_auth_header": ""},
	})
	i, _ := st.Integrations(ctx)
	if i.VLAuthHeader != "" {
		t.Fatalf("explicit empty must clear the secret, got %q", i.VLAuthHeader)
	}
}

func TestPutSettingsValidation(t *testing.T) {
	h, _, _ := newTestAPIWithSettings(t)
	for name, body := range map[string]map[string]any{
		"bad vm url":     {"integrations": map[string]any{"vm_enabled": true, "vm_url": "ftp://vm"}},
		"enabled no url": {"integrations": map[string]any{"vm_enabled": true, "vm_url": ""}},
		"zero retention": {"general": map[string]any{"retention_days_results": 0}},
		"bad log level":  {"general": map[string]any{"log_level": "shout"}},
		"bad label name": {"integrations": map[string]any{"vm_extra_labels": map[string]string{"1bad": "x"}}},
	} {
		rec := do(t, h, http.MethodPut, "/api/v1/settings", body)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: status %d, want 400 (%s)", name, rec.Code, rec.Body)
		}
		var e errorBody
		json.Unmarshal(rec.Body.Bytes(), &e)
		if e.Error.Code != "invalid_request" || e.Error.Message == "" {
			t.Fatalf("%s: envelope = %+v", name, e)
		}
	}
}

// TestPutSettingsRejectsEnablingWithoutEffectiveURL is the regression case
// for the finding that enabling an integration while leaving its stored URL
// empty (and not supplying a new one in the same request) used to pass
// validation, since only a body-supplied URL was checked.
func TestPutSettingsRejectsEnablingWithoutEffectiveURL(t *testing.T) {
	h, _, _ := newTestAPIWithSettings(t)
	rec := do(t, h, http.MethodPut, "/api/v1/settings", map[string]any{
		"integrations": map[string]any{"vm_enabled": true},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400 (%s)", rec.Code, rec.Body)
	}
}

func TestPutSettingsOnlyTouchesProvidedSections(t *testing.T) {
	h, _, st := newTestAPIWithSettings(t)
	ctx := context.Background()
	before, _ := st.Engines(ctx)
	do(t, h, http.MethodPut, "/api/v1/settings", map[string]any{
		"general": map[string]any{"units": "MB/s"},
	})
	after, _ := st.Engines(ctx)
	if after.SpeedtestBin != before.SpeedtestBin {
		t.Fatal("a general-only PUT rewrote the engines section")
	}
	g, _ := st.General(ctx)
	if g.Units != "MB/s" {
		t.Fatalf("units = %q", g.Units)
	}
}

func TestSettingsTestVMProbesHealth(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	h, _, st := newTestAPIWithSettings(t)
	ctx := context.Background()
	st.Set(ctx, settings.KeyVMURL, srv.URL)
	st.Set(ctx, settings.KeyVMAuthHeader, "Bearer tok")

	rec := do(t, h, http.MethodPost, "/api/v1/settings/test/vm", map[string]any{})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var body struct {
		OK     bool   `json:"ok"`
		Status int    `json:"status"`
		Error  string `json:"error"`
	}
	json.Unmarshal(rec.Body.Bytes(), &body)
	if !body.OK || body.Status != 200 {
		t.Fatalf("probe = %+v", body)
	}
	if path != "/health" {
		t.Fatalf("probed %q, want /health", path)
	}
}

// TestSettingsTestTrimsTrailingSlash is the regression case for the finding
// that a stored/body URL with a trailing slash produced a "//health" probe
// path.
func TestSettingsTestTrimsTrailingSlash(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	h, _, _ := newTestAPIWithSettings(t)
	rec := do(t, h, http.MethodPost, "/api/v1/settings/test/vm", map[string]any{"url": srv.URL + "/"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if path != "/health" {
		t.Fatalf("path = %q, want /health", path)
	}
}

func TestSettingsTestUsesBodyURLAndKeepsStoredSecret(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	h, _, st := newTestAPIWithSettings(t)
	ctx := context.Background()
	// The stored secret is only reused when the body's URL is the same
	// origin as the stored one.
	st.Set(ctx, settings.KeyVLURL, srv.URL)
	st.Set(ctx, settings.KeyVLAuthHeader, "Bearer stored")
	rec := do(t, h, http.MethodPost, "/api/v1/settings/test/vl", map[string]any{
		"url": srv.URL, "auth_header": settings.MaskedSecret,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if auth != "Bearer stored" {
		t.Fatalf("auth = %q, want the stored secret", auth)
	}
}

// TestSettingsTestDoesNotLeakStoredSecretToOtherHost is the regression case
// for the SSRF/credential-exfil finding: a caller-supplied URL pointing at a
// different origin than the stored one must never receive the stored
// Authorization header, whether auth_header is omitted or echoes the mask.
func TestSettingsTestDoesNotLeakStoredSecretToOtherHost(t *testing.T) {
	var auth string
	var sawAuth bool
	attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, sawAuth = r.Header.Get("Authorization"), r.Header.Get("Authorization") != ""
		w.WriteHeader(http.StatusOK)
	}))
	defer attacker.Close()
	h, _, st := newTestAPIWithSettings(t)
	ctx := context.Background()
	st.Set(ctx, settings.KeyVLURL, "http://vl.internal:9428")
	st.Set(ctx, settings.KeyVLAuthHeader, "Bearer supersecret")

	for name, body := range map[string]map[string]any{
		"auth_header omitted": {"url": attacker.URL},
		"auth_header masked":  {"url": attacker.URL, "auth_header": settings.MaskedSecret},
	} {
		sawAuth = false
		rec := do(t, h, http.MethodPost, "/api/v1/settings/test/vl", body)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d", name, rec.Code)
		}
		if sawAuth {
			t.Fatalf("%s: stored secret leaked to a different origin: %q", name, auth)
		}
	}
}

func TestSettingsTestReportsUnreachable(t *testing.T) {
	h, _, _ := newTestAPIWithSettings(t)
	rec := do(t, h, http.MethodPost, "/api/v1/settings/test/vm", map[string]any{"url": "http://127.0.0.1:1"})
	if rec.Code != http.StatusOK {
		t.Fatalf("an unreachable endpoint is still a successful API call, got %d", rec.Code)
	}
	var body struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	json.Unmarshal(rec.Body.Bytes(), &body)
	if body.OK || body.Error == "" {
		t.Fatalf("probe = %+v", body)
	}
}

func TestSettingsTestRejectsUnknownTargetAndBadURL(t *testing.T) {
	h, _, _ := newTestAPIWithSettings(t)
	if rec := do(t, h, http.MethodPost, "/api/v1/settings/test/notify", map[string]any{}); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown target = %d, want 404", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, "/api/v1/settings/test/vm", map[string]any{"url": "ftp://x"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad url = %d, want 400", rec.Code)
	}
}

// settingsAPIOption tweaks Deps for newSettingsAPI.
type settingsAPIOption func(*Deps)

// withNotifier installs a ChannelTester for POST
// /api/v1/settings/test/notify/{channel_id}.
func withNotifier(ct ChannelTester) settingsAPIOption {
	return func(d *Deps) { d.Notifier = ct }
}

// newSettingsAPI is newTestAPIWithSettings with room for options such as
// withNotifier.
func newSettingsAPI(t *testing.T, opts ...settingsAPIOption) (http.Handler, *settings.Store) {
	t.Helper()
	var st *settings.Store
	h, _, _ := newTestAPIWith(t, func(d *Deps) {
		s, err := settings.New(context.Background(), d.Store)
		if err != nil {
			t.Fatal(err)
		}
		st = s
		d.Settings = s
		for _, opt := range opts {
			opt(d)
		}
	})
	return h, st
}

// stubTester is a ChannelTester that always returns err (nil means success).
type stubTester struct{ err error }

func (s stubTester) TestChannel(context.Context, settings.Channel) error { return s.err }

func TestGetSettingsMasksChannelTokens(t *testing.T) {
	h, st := newSettingsAPI(t)
	st.Set(context.Background(), settings.KeyNotifyChannels, []settings.Channel{
		{ID: "c1", Type: "ntfy", URL: "https://ntfy.sh/x", Token: "tk_1"},
		{ID: "c2", Type: "webhook", URL: "https://hook"},
	})
	var body struct {
		Notifications settings.Notifications `json:"notifications"`
	}
	rec := do(t, h, http.MethodGet, "/api/v1/settings", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Notifications.Channels[0].Token != settings.MaskedSecret {
		t.Errorf("token = %q, want masked", body.Notifications.Channels[0].Token)
	}
	if body.Notifications.Channels[1].Token != "" {
		t.Errorf("unset token = %q, want empty", body.Notifications.Channels[1].Token)
	}
}

func TestPutNotificationsKeepsMaskedTokenByID(t *testing.T) {
	h, st := newSettingsAPI(t)
	st.Set(context.Background(), settings.KeyNotifyChannels, []settings.Channel{
		{ID: "c1", Type: "ntfy", URL: "https://ntfy.sh/x", Token: "tk_1"}})
	rec := do(t, h, http.MethodPut, "/api/v1/settings", map[string]any{
		"notifications": map[string]any{"channels": []map[string]any{
			{"id": "c1", "type": "ntfy", "url": "https://ntfy.sh/x", "token": settings.MaskedSecret}}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT = %d body=%s", rec.Code, rec.Body)
	}
	got, _ := st.Notifications(context.Background())
	if got.Channels[0].Token != "tk_1" || got.Channels[0].URL != "https://ntfy.sh/x" {
		t.Fatalf("channel = %+v, want the stored token kept", got.Channels[0])
	}
}

// TestPutNotificationsRejectsMaskedTokenAfterURLChange is the regression
// case for finding 4: echoing back "***" while also changing the url (or
// type) must not forward the stored token to whatever host the new url
// points at. It must be rejected as a 400, and the PUT must be a no-op —
// not even other, unrelated sections may be written.
func TestPutNotificationsRejectsMaskedTokenAfterURLChange(t *testing.T) {
	h, st := newSettingsAPI(t)
	ctx := context.Background()
	st.Set(ctx, settings.KeyNotifyChannels, []settings.Channel{
		{ID: "c1", Type: "ntfy", URL: "https://ntfy.sh/x", Token: "tk_1"}})
	st.Set(ctx, settings.KeyNotifyCooldownMinutes, 30)

	rec := do(t, h, http.MethodPut, "/api/v1/settings", map[string]any{
		"notifications": map[string]any{
			"cooldown_minutes": 45,
			"channels": []map[string]any{
				{"id": "c1", "type": "ntfy", "url": "https://attacker.example/x", "token": settings.MaskedSecret}},
		},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400: %s", rec.Code, rec.Body)
	}
	got, _ := st.Notifications(ctx)
	if got.Channels[0].URL != "https://ntfy.sh/x" || got.CooldownMinutes != 30 {
		t.Fatalf("rejected PUT was not a no-op: %+v", got)
	}
}

// TestGetSettingsMasksChannelHeadersAndApprisURLs is the regression case
// for finding 1: webhook header values and apprise urls (which embed
// credentials) must be masked in GET /settings just like the token is,
// and a PUT echoing the masks back must restore them from the stored
// channel.
func TestGetSettingsMasksChannelHeadersAndApprisURLs(t *testing.T) {
	h, st := newSettingsAPI(t)
	ctx := context.Background()
	st.Set(ctx, settings.KeyNotifyChannels, []settings.Channel{
		{ID: "wh", Type: "webhook", URL: "https://hook", Headers: map[string]string{"Authorization": "secret-header"}},
		{ID: "ap", Type: "apprise", URL: "https://apprise", URLs: []string{"tgram://token/chat"}},
	})

	rec := do(t, h, http.MethodGet, "/api/v1/settings", nil)
	var body struct {
		Notifications settings.Notifications `json:"notifications"`
	}
	json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Notifications.Channels[0].Headers["Authorization"] != settings.MaskedSecret {
		t.Fatalf("header = %+v, want masked", body.Notifications.Channels[0].Headers)
	}
	if body.Notifications.Channels[1].URLs[0] != settings.MaskedSecret {
		t.Fatalf("apprise url = %+v, want masked", body.Notifications.Channels[1].URLs)
	}

	rec = do(t, h, http.MethodPut, "/api/v1/settings", map[string]any{
		"notifications": map[string]any{"channels": []map[string]any{
			{"id": "wh", "type": "webhook", "url": "https://hook",
				"headers": map[string]string{"Authorization": settings.MaskedSecret}},
			{"id": "ap", "type": "apprise", "url": "https://apprise",
				"urls": []string{settings.MaskedSecret}},
		}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT = %d body=%s", rec.Code, rec.Body)
	}
	got, _ := st.Notifications(ctx)
	if got.Channels[0].Headers["Authorization"] != "secret-header" {
		t.Fatalf("header not restored: %+v", got.Channels[0].Headers)
	}
	if got.Channels[1].URLs[0] != "tgram://token/chat" {
		t.Fatalf("apprise url not restored: %+v", got.Channels[1].URLs)
	}
}

func TestPutNotificationsValidation(t *testing.T) {
	h, _ := newSettingsAPI(t)
	for _, tc := range []struct {
		name, want string
		body       map[string]any
	}{
		{"bad channel type", "type", map[string]any{"channels": []map[string]any{
			{"id": "c1", "type": "pigeon", "url": "https://x"}}}},
		{"duplicate id", "duplicate", map[string]any{"channels": []map[string]any{
			{"id": "c1", "type": "ntfy", "url": "https://x"},
			{"id": "c1", "type": "ntfy", "url": "https://y"}}}},
		{"cooldown", "cooldown_minutes", map[string]any{"cooldown_minutes": 0}},
		{"quiet hours", "quiet_hours", map[string]any{"quiet_hours_start": "25:00"}},
		{"half-set quiet hours", "quiet_hours", map[string]any{"quiet_hours_start": "22:00"}},
		{"negative threshold", "download_mbps_min", map[string]any{
			"default_thresholds": map[string]any{"download_mbps_min": -1}}},
	} {
		rec := do(t, h, http.MethodPut, "/api/v1/settings",
			map[string]any{"notifications": tc.body})
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), tc.want) {
			t.Errorf("%s: %d body=%s, want 400 mentioning %q", tc.name, rec.Code, rec.Body, tc.want)
		}
	}
}

func TestTestNotifyChannel(t *testing.T) {
	h, st := newSettingsAPI(t, withNotifier(stubTester{err: errors.New("connection refused")}))
	st.Set(context.Background(), settings.KeyNotifyChannels, []settings.Channel{
		{ID: "c1", Type: "ntfy", URL: "https://ntfy.sh/x"}})

	var body struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	rec := do(t, h, http.MethodPost, "/api/v1/settings/test/notify/c1", nil)
	json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusOK || body.OK || !strings.Contains(body.Error, "connection refused") {
		t.Fatalf("= %d %+v, want 200 with ok=false and the reason inline", rec.Code, body)
	}

	rec = do(t, h, http.MethodPost, "/api/v1/settings/test/notify/nope", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown channel = %d, want 404", rec.Code)
	}
}

func TestTestUnknownTargetStillRejected(t *testing.T) {
	h, _ := newSettingsAPI(t)
	if rec := do(t, h, http.MethodPost, "/api/v1/settings/test/notify", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("POST /settings/test/notify = %d, want 404 (it is not a test target)", rec.Code)
	}
}

func TestGetSettingsIncludesAuthAndLockedKeys(t *testing.T) {
	h, st := newSettingsAPI(t, withEnv("ST_LOCK_ENV=true", "ST_AUTH_MODE=open"))
	_ = st
	var body struct {
		Auth   settings.Auth `json:"auth"`
		Locked []string      `json:"locked"`
	}
	doJSON(t, h, http.MethodGet, "/api/v1/settings", nil, &body)
	if body.Auth.Mode != settings.AuthModeOpen || body.Auth.TrustedProxies == nil {
		t.Fatalf("auth = %+v", body.Auth)
	}
	if !slices.Contains(body.Locked, settings.KeyAuthMode) {
		t.Fatalf("locked = %v, want auth.mode", body.Locked)
	}
}

func TestPutRejectsLockedKeys(t *testing.T) {
	h, _ := newSettingsAPI(t, withEnv("ST_LOCK_ENV=true", "ST_AUTH_MODE=open"))
	rec := doJSON(t, h, http.MethodPut, "/api/v1/settings",
		map[string]any{"auth": map[string]any{"mode": "token"}}, nil)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "environment") {
		t.Fatalf("= %d %s, want 400 saying the key is set by the environment", rec.Code, rec.Body)
	}
	// An unlocked key in the same section still saves.
	if rec := doJSON(t, h, http.MethodPut, "/api/v1/settings",
		map[string]any{"auth": map[string]any{"admin_group": "admins"}}, nil); rec.Code != http.StatusOK {
		t.Fatalf("unlocked key = %d %s", rec.Code, rec.Body)
	}
}

func TestPutAuthValidation(t *testing.T) {
	h, _ := newSettingsAPI(t)
	for _, tc := range []struct {
		name, want string
		body       map[string]any
	}{
		{"bad mode", "mode", map[string]any{"mode": "openish"}},
		{"bad cidr", "trusted_proxies", map[string]any{"trusted_proxies": []string{"10.0.0.0/33"}}},
		{"bad header name", "user_header", map[string]any{"user_header": "Remote User"}},
		{"empty separator", "groups_separator", map[string]any{"groups_separator": ""}},
	} {
		rec := doJSON(t, h, http.MethodPut, "/api/v1/settings", map[string]any{"auth": tc.body}, nil)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), tc.want) {
			t.Errorf("%s: %d %s, want 400 mentioning %q", tc.name, rec.Code, rec.Body, tc.want)
		}
	}
}

// TestPutAuthRejectsWeakHeaderNames is a regression test for review item 9:
// header-name validation must use the RFC 7230 tchar set, not the weaker
// CanonicalHeaderKey + blacklist check that let a comma-separated name like
// "Remote,User" through.
func TestPutAuthRejectsWeakHeaderNames(t *testing.T) {
	h, _ := newSettingsAPI(t)
	for _, bad := range []string{"Remote,User", "Remote User", "Remote:User", "Remote\tUser"} {
		rec := doJSON(t, h, http.MethodPut, "/api/v1/settings",
			map[string]any{"auth": map[string]any{"user_header": bad}}, nil)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("user_header=%q = %d %s, want 400", bad, rec.Code, rec.Body)
		}
	}
	if rec := doJSON(t, h, http.MethodPut, "/api/v1/settings",
		map[string]any{"auth": map[string]any{"user_header": "X-Remote-User"}}, nil); rec.Code != http.StatusOK {
		t.Errorf("valid header name = %d %s, want 200", rec.Code, rec.Body)
	}
}

func TestSwitchToForwardAuthLockoutGuard(t *testing.T) {
	h, _ := newSettingsAPI(t)
	body := map[string]any{"auth": map[string]any{"mode": settings.AuthModeForward,
		"user_header": "Remote-User", "trusted_proxies": []string{}}}
	rec := doJSON(t, h, http.MethodPut, "/api/v1/settings", body, nil)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "trusted_proxies") {
		t.Fatalf("no proxies = %d %s, want 400", rec.Code, rec.Body)
	}

	body["auth"].(map[string]any)["trusted_proxies"] = []string{"192.0.2.0/24"}
	rec = doJSON(t, h, http.MethodPut, "/api/v1/settings", body, nil) // request carries no header
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Remote-User") {
		t.Fatalf("no header on the switching request = %d %s, want 400 naming the header", rec.Code, rec.Body)
	}

	req := jsonRequest(t, http.MethodPut, "/api/v1/settings", body)
	req.RemoteAddr = "192.0.2.5:1"
	req.Header.Set("Remote-User", "alice")
	out := httptest.NewRecorder()
	h.ServeHTTP(out, req)
	if out.Code != http.StatusOK {
		t.Fatalf("valid switch = %d %s", out.Code, out.Body)
	}
}

func TestSwitchToTokenModeRequiresAToken(t *testing.T) {
	h, _ := newSettingsAPI(t)
	rec := doJSON(t, h, http.MethodPut, "/api/v1/settings",
		map[string]any{"auth": map[string]any{"mode": settings.AuthModeToken}}, nil)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "token") {
		t.Fatalf("= %d %s, want 400 — switching to token mode with no token issued locks everyone out", rec.Code, rec.Body)
	}

	var created struct {
		Token string `json:"token"`
	}
	doJSON(t, h, http.MethodPost, "/api/v1/settings/tokens", map[string]any{"name": "a"}, &created)

	// A token now exists, but this switching request carries none itself.
	rec = doJSON(t, h, http.MethodPut, "/api/v1/settings",
		map[string]any{"auth": map[string]any{"mode": settings.AuthModeToken}}, nil)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "bearer token") {
		t.Fatalf("no bearer token on the switching request = %d %s, want 400 naming a bearer token", rec.Code, rec.Body)
	}

	req := jsonRequest(t, http.MethodPut, "/api/v1/settings",
		map[string]any{"auth": map[string]any{"mode": settings.AuthModeToken}})
	req.Header.Set("Authorization", "Bearer "+created.Token)
	out := httptest.NewRecorder()
	h.ServeHTTP(out, req)
	if out.Code != http.StatusOK {
		t.Fatalf("with a valid bearer token on the request = %d %s", out.Code, out.Body)
	}
}

// TestTokenIdentityCannotChangeAuthSection is a regression test for review
// item 3's RULING: a bearer/query API token is full-access for the normal
// API, but must not be usable to write the Auth section itself — only a
// forward-auth or open-mode session may.
func TestTokenIdentityCannotChangeAuthSection(t *testing.T) {
	var mw *auth.Middleware
	var plain string
	h, _, _ := newTestAPIWith(t, func(d *Deps) {
		s, err := settings.New(context.Background(), d.Store)
		if err != nil {
			t.Fatal(err)
		}
		d.Settings = s
		mw = auth.New(nil, storeTokenLookup{db: d.Store}, time.Now)
		if err := mw.Configure(settings.Auth{Mode: settings.AuthModeToken}); err != nil {
			t.Fatal(err)
		}
		d.Auth = authAdapter{mw: mw, mode: settings.AuthModeToken}

		p, hash, prefix, err := auth.GenerateToken()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := d.Store.CreateAPIToken(context.Background(), "test", hash, prefix); err != nil {
			t.Fatal(err)
		}
		plain = p
	})
	t.Cleanup(mw.Close)

	req := jsonRequest(t, http.MethodPut, "/api/v1/settings",
		map[string]any{"auth": map[string]any{"admin_group": "admins"}})
	req.Header.Set("Authorization", "Bearer "+plain)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "forward-auth or open-mode") {
		t.Fatalf("= %d %s, want 403 naming forward-auth/open-mode", rec.Code, rec.Body)
	}

	// A non-auth section write with the same token identity is unaffected.
	req2 := jsonRequest(t, http.MethodPut, "/api/v1/settings",
		map[string]any{"general": map[string]any{"units": "MB/s"}})
	req2.Header.Set("Authorization", "Bearer "+plain)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("non-auth section with a token = %d %s, want 200", rec2.Code, rec2.Body)
	}
}

// TestForwardAuthNarrowingTrustedProxiesIsGuarded is a regression test for
// review item 4: changing trusted_proxies or user_header while already in
// forward_auth must be guarded even though the mode itself doesn't change
// — otherwise an operator could narrow the config to exclude their own
// request and lock themselves out.
func TestForwardAuthNarrowingTrustedProxiesIsGuarded(t *testing.T) {
	h, st := newSettingsAPI(t)
	ctx := context.Background()
	st.Set(ctx, settings.KeyAuthMode, settings.AuthModeForward)
	st.Set(ctx, settings.KeyAuthUserHeader, "Remote-User")
	st.Set(ctx, settings.KeyAuthTrustedProxies, []string{"10.0.0.0/8"})

	// Narrowing trusted_proxies from a request outside the new range must
	// be rejected, even though mode is unchanged.
	rec := doJSON(t, h, http.MethodPut, "/api/v1/settings",
		map[string]any{"auth": map[string]any{"trusted_proxies": []string{"192.0.2.0/24"}}}, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("narrowing trusted_proxies from an untrusted request = %d %s, want 400", rec.Code, rec.Body)
	}

	// The same change, from a request that does satisfy the new config, is
	// allowed.
	req := jsonRequest(t, http.MethodPut, "/api/v1/settings",
		map[string]any{"auth": map[string]any{"trusted_proxies": []string{"192.0.2.0/24"}}})
	req.RemoteAddr = "192.0.2.5:1"
	req.Header.Set("Remote-User", "alice")
	out := httptest.NewRecorder()
	h.ServeHTTP(out, req)
	if out.Code != http.StatusOK {
		t.Fatalf("narrowing from a satisfying request = %d %s, want 200", out.Code, out.Body)
	}
}

func TestSwitchingAwayFromForwardAuthIsAlwaysAllowed(t *testing.T) {
	h, st := newSettingsAPI(t)
	st.Set(context.Background(), settings.KeyAuthMode, settings.AuthModeForward)
	if rec := doJSON(t, h, http.MethodPut, "/api/v1/settings",
		map[string]any{"auth": map[string]any{"mode": settings.AuthModeOpen}}, nil); rec.Code != http.StatusOK {
		t.Fatalf("= %d %s, want the escape hatch to always work", rec.Code, rec.Body)
	}
}
