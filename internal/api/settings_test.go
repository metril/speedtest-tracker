package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/metril/speedtest-tracker/internal/settings"
	"github.com/metril/speedtest-tracker/internal/store"
)

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
