package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/metril/speedtest-tracker/internal/auth"
	"github.com/metril/speedtest-tracker/internal/settings"
	"github.com/metril/speedtest-tracker/internal/store"
)

func TestCreateTokenReturnsPlaintextOnce(t *testing.T) {
	h, _ := newSettingsAPI(t)
	var created struct {
		ID     int64  `json:"id"`
		Name   string `json:"name"`
		Token  string `json:"token"`
		Prefix string `json:"prefix"`
	}
	rec := doJSON(t, h, http.MethodPost, "/api/v1/settings/tokens",
		map[string]any{"name": "home assistant"}, &created)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST = %d %s", rec.Code, rec.Body)
	}
	if !strings.HasPrefix(created.Token, "stt_") || !strings.HasPrefix(created.Token, created.Prefix) {
		t.Fatalf("created = %+v", created)
	}

	var list struct {
		Tokens []store.APIToken `json:"tokens"`
	}
	listRec := doJSON(t, h, http.MethodGet, "/api/v1/settings/tokens", nil, &list)
	if len(list.Tokens) != 1 || list.Tokens[0].Prefix != created.Prefix {
		t.Fatalf("list = %+v", list.Tokens)
	}
	if strings.Contains(listRec.Body.String(), created.Token) {
		t.Fatal("the plaintext token is listable; it must be shown exactly once, at creation")
	}
}

func TestCreateTokenValidatesName(t *testing.T) {
	h, _ := newSettingsAPI(t)
	for _, body := range []map[string]any{{}, {"name": "   "}, {"name": strings.Repeat("x", 200)}} {
		rec := doJSON(t, h, http.MethodPost, "/api/v1/settings/tokens", body, nil)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "name") {
			t.Errorf("%v = %d %s, want 400 mentioning name", body, rec.Code, rec.Body)
		}
	}
}

func TestDeleteToken(t *testing.T) {
	h, _ := newSettingsAPI(t)
	var created struct {
		ID int64 `json:"id"`
	}
	doJSON(t, h, http.MethodPost, "/api/v1/settings/tokens", map[string]any{"name": "a"}, &created)
	if rec := do(t, h, http.MethodDelete, fmt.Sprintf("/api/v1/settings/tokens/%d", created.ID), nil); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE = %d", rec.Code)
	}
	if rec := do(t, h, http.MethodDelete, fmt.Sprintf("/api/v1/settings/tokens/%d", created.ID), nil); rec.Code != http.StatusNotFound {
		t.Fatalf("second DELETE = %d, want 404", rec.Code)
	}
	if rec := do(t, h, http.MethodDelete, "/api/v1/settings/tokens/abc", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("non-numeric id = %d, want 400", rec.Code)
	}
}

// TestTokenRoutesRequireAdmin is a regression test for review item 2: a
// non-admin forward_auth caller must not be able to list, create or revoke
// API tokens — a minted token always resolves to an admin identity, so
// letting a non-admin mint one is a privilege escalation.
func TestTokenRoutesRequireAdmin(t *testing.T) {
	var mw *auth.Middleware
	h, _, _ := newTestAPIWith(t, func(d *Deps) {
		s, err := settings.New(context.Background(), d.Store)
		if err != nil {
			t.Fatal(err)
		}
		d.Settings = s
		mw = auth.New(nil, storeTokenLookup{db: d.Store}, time.Now)
		cfg := settings.Auth{Mode: settings.AuthModeForward, UserHeader: "Remote-User",
			GroupsHeader: "Remote-Groups", GroupsSeparator: ",", AdminGroup: "admins",
			TrustedProxies: []string{"192.0.2.0/24"}}
		if err := mw.Configure(cfg); err != nil {
			t.Fatal(err)
		}
		d.Auth = authAdapter{mw: mw, mode: cfg.Mode}
	})
	t.Cleanup(mw.Close)

	nonAdmin := func(method, path string, body any) *httptest.ResponseRecorder {
		req := jsonRequest(t, method, path, body)
		req.RemoteAddr = "192.0.2.9:1"
		req.Header.Set("Remote-User", "bob")
		req.Header.Set("Remote-Groups", "users") // not in the admins group
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	if rec := nonAdmin(http.MethodGet, "/api/v1/settings/tokens", nil); rec.Code != http.StatusForbidden {
		t.Errorf("GET tokens as non-admin = %d %s, want 403", rec.Code, rec.Body)
	}
	if rec := nonAdmin(http.MethodPost, "/api/v1/settings/tokens", map[string]any{"name": "x"}); rec.Code != http.StatusForbidden {
		t.Errorf("POST tokens as non-admin = %d %s, want 403", rec.Code, rec.Body)
	}
	if rec := nonAdmin(http.MethodDelete, "/api/v1/settings/tokens/1", nil); rec.Code != http.StatusForbidden {
		t.Errorf("DELETE tokens as non-admin = %d %s, want 403", rec.Code, rec.Body)
	}
}

func TestDeletingTheLastTokenInTokenModeIsRefused(t *testing.T) {
	h, st := newSettingsAPI(t)
	st.Set(context.Background(), settings.KeyAuthMode, settings.AuthModeToken)
	var created struct {
		ID int64 `json:"id"`
	}
	doJSON(t, h, http.MethodPost, "/api/v1/settings/tokens", map[string]any{"name": "only"}, &created)
	rec := do(t, h, http.MethodDelete, fmt.Sprintf("/api/v1/settings/tokens/%d", created.ID), nil)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "last") {
		t.Fatalf("= %d %s, want 400 — revoking the only token in token mode locks everyone out", rec.Code, rec.Body)
	}
}
