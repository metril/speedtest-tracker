package settings

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/metril/speedtest-tracker/internal/store"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, _ := newTestSettings(t)
	return s
}

// newTestSettings returns a Store plus the underlying db.Store, for tests
// that need to reach into the raw table (e.g. deleting a row).
func newTestSettings(t *testing.T) (*Store, *store.Store) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	s, err := New(context.Background(), db)
	if err != nil {
		t.Fatalf("settings.New: %v", err)
	}
	return s, db
}

func TestNewSeedsGeneralDefaults(t *testing.T) {
	s := newTestStore(t)
	g, err := s.General(context.Background())
	if err != nil {
		t.Fatalf("General: %v", err)
	}
	if g.Timezone != "UTC" {
		t.Errorf("Timezone = %q, want UTC", g.Timezone)
	}
	if g.Units != "Mbps" {
		t.Errorf("Units = %q, want Mbps", g.Units)
	}
	if g.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want info", g.LogLevel)
	}
	if g.RetentionDaysResults != 90 {
		t.Errorf("RetentionDaysResults = %d, want 90", g.RetentionDaysResults)
	}
	if g.RetentionDaysRuns != 90 {
		t.Errorf("RetentionDaysRuns = %d, want 90", g.RetentionDaysRuns)
	}
}

func TestSetGetRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	if err := s.Set(ctx, "general.timezone", "Europe/London"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	raw, ok, err := s.Get(ctx, "general.timezone")
	if err != nil || !ok {
		t.Fatalf("Get: raw=%s ok=%v err=%v", raw, ok, err)
	}
	if string(raw) != `"Europe/London"` {
		t.Errorf("raw = %s, want \"Europe/London\"", raw)
	}
	g, err := s.General(ctx)
	if err != nil {
		t.Fatalf("General: %v", err)
	}
	if g.Timezone != "Europe/London" {
		t.Errorf("Timezone = %q", g.Timezone)
	}
}

func TestGetMissingKey(t *testing.T) {
	_, ok, err := newTestStore(t).Get(context.Background(), "general.nope")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if ok {
		t.Error("ok = true for missing key, want false")
	}
}

func TestSubscribeReceivesChangedKey(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	ch, cancel := s.Subscribe()
	defer cancel()

	if err := s.Set(ctx, "general.log_level", "debug"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	select {
	case key := <-ch:
		if key != "general.log_level" {
			t.Errorf("key = %q, want general.log_level", key)
		}
	case <-time.After(time.Second):
		t.Fatal("no notification within 1s")
	}
}

func TestSubscribeDoesNotBlockOnSlowSubscriber(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	_, cancel := s.Subscribe() // never drained
	defer cancel()

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			s.Set(ctx, "general.base_url", "http://x")
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Set blocked on a slow subscriber")
	}
}

func TestEnginesDefaults(t *testing.T) {
	s := newTestStore(t)
	got, err := s.Engines(context.Background())
	if err != nil {
		t.Fatalf("Engines: %v", err)
	}
	if got.SpeedtestBin != "speedtest" || got.Iperf3Bin != "iperf3" {
		t.Errorf("bins = %q/%q, want speedtest/iperf3", got.SpeedtestBin, got.Iperf3Bin)
	}
	if !got.OoklaAcceptLicense || !got.OoklaAcceptGDPR {
		t.Errorf("license/gdpr = %v/%v, want true/true", got.OoklaAcceptLicense, got.OoklaAcceptGDPR)
	}
	if got.ServerListTTLSeconds != 86400 {
		t.Errorf("ttl = %d, want 86400", got.ServerListTTLSeconds)
	}
	for name, raw := range map[string]json.RawMessage{
		"ookla":      got.DefaultOoklaOptions,
		"cloudflare": got.DefaultCloudflareOptions,
		"iperf3":     got.DefaultIperf3Options,
	} {
		if string(raw) != "{}" {
			t.Errorf("default %s options = %q, want {}", name, raw)
		}
	}
}

func TestGeneralFallsBackToDefaultOnMissingKey(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	if _, err := s.db.Write.ExecContext(ctx, `DELETE FROM settings WHERE key=?`, KeyTimezone); err != nil {
		t.Fatalf("delete row: %v", err)
	}
	g, err := s.General(ctx)
	if err != nil {
		t.Fatalf("General: %v", err)
	}
	if g.Timezone != "UTC" {
		t.Errorf("Timezone = %q, want the seeded default UTC", g.Timezone)
	}
}

func TestIntegrationsDefaults(t *testing.T) {
	s := newTestStore(t)
	got, err := s.Integrations(context.Background())
	if err != nil {
		t.Fatalf("Integrations: %v", err)
	}
	if got.VMEnabled || got.VLEnabled || got.MetricsEnabled {
		t.Fatalf("integrations default to disabled, got %+v", got)
	}
	if got.VMExtraLabels == nil || len(got.VMExtraLabels) != 0 {
		t.Fatalf("vm_extra_labels = %v, want empty non-nil map", got.VMExtraLabels)
	}
	if got.VLStreamFields == nil || len(got.VLStreamFields) != 0 {
		t.Fatalf("vl_stream_fields = %v, want empty non-nil map", got.VLStreamFields)
	}
}

func TestIntegrationsRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	for key, val := range map[string]any{
		KeyVMEnabled:     true,
		KeyVMURL:         "http://vm:8428",
		KeyVMAuthHeader:  "Bearer tok",
		KeyVMExtraLabels: map[string]string{"host": "pi4"},
	} {
		if err := s.Set(ctx, key, val); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Integrations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !got.VMEnabled || got.VMURL != "http://vm:8428" || got.VMAuthHeader != "Bearer tok" ||
		got.VMExtraLabels["host"] != "pi4" {
		t.Fatalf("round trip = %+v", got)
	}
}

func TestGeneralPruneIntervalDefault(t *testing.T) {
	s := newTestStore(t)
	g, err := s.General(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if g.RetentionPruneIntervalMinutes != 60 {
		t.Fatalf("prune interval = %d, want 60", g.RetentionPruneIntervalMinutes)
	}
	if g.RetentionDaysResults != 90 || g.RetentionDaysRuns != 90 {
		t.Fatalf("retention = %d/%d, want 90/90", g.RetentionDaysResults, g.RetentionDaysRuns)
	}
}

func TestEnginesReflectsOverride(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.Set(ctx, KeySpeedtestBin, "/opt/ookla/speedtest"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := s.Engines(ctx)
	if err != nil {
		t.Fatalf("Engines: %v", err)
	}
	if got.SpeedtestBin != "/opt/ookla/speedtest" {
		t.Errorf("SpeedtestBin = %q", got.SpeedtestBin)
	}
}

func TestNotificationsDefaults(t *testing.T) {
	s := newTestStore(t)
	got, err := s.Notifications(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Enabled {
		t.Fatalf("notifications default to disabled, got %+v", got)
	}
	if got.Channels == nil || len(got.Channels) != 0 {
		t.Fatalf("channels = %v, want empty non-nil slice", got.Channels)
	}
	if got.CooldownMinutes != 60 {
		t.Fatalf("cooldown = %d, want 60", got.CooldownMinutes)
	}
	if !got.NotifyRecovery {
		t.Fatal("notify_recovery defaults to true")
	}
	if got.QuietHoursStart != "" || got.QuietHoursEnd != "" {
		t.Fatalf("quiet hours default to empty, got %q-%q", got.QuietHoursStart, got.QuietHoursEnd)
	}
	if got.DefaultThresholds.DownloadMbpsMin != nil {
		t.Fatalf("default thresholds start unset, got %+v", got.DefaultThresholds)
	}
}

func TestNotificationsRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	chans := []Channel{{
		ID: "c1", Type: "ntfy", Name: "phone", Enabled: true,
		URL: "https://ntfy.sh/speedtest", Token: "tk_1", Priority: "high", Tags: []string{"warning"},
	}}
	min := 100.0
	for key, val := range map[string]any{
		KeyNotifyEnabled:           true,
		KeyNotifyChannels:          chans,
		KeyNotifyDefaultThresholds: Thresholds{DownloadMbpsMin: &min},
		KeyNotifyCooldownMinutes:   15,
		KeyNotifyQuietStart:        "22:00",
		KeyNotifyQuietEnd:          "07:00",
	} {
		if err := s.Set(ctx, key, val); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Notifications(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Enabled || got.CooldownMinutes != 15 || got.QuietHoursStart != "22:00" {
		t.Fatalf("round trip = %+v", got)
	}
	if len(got.Channels) != 1 || got.Channels[0].Token != "tk_1" || got.Channels[0].Tags[0] != "warning" {
		t.Fatalf("channels = %+v", got.Channels)
	}
	if got.DefaultThresholds.DownloadMbpsMin == nil || *got.DefaultThresholds.DownloadMbpsMin != 100 {
		t.Fatalf("default thresholds = %+v", got.DefaultThresholds)
	}
}

func TestNotificationsCooldownClampedToOne(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.Set(ctx, KeyNotifyCooldownMinutes, 0); err != nil {
		t.Fatal(err)
	}
	got, err := s.Notifications(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.CooldownMinutes != 1 {
		t.Fatalf("cooldown = %d, want clamped to 1", got.CooldownMinutes)
	}
}

func TestAuthDefaults(t *testing.T) {
	st, _ := newTestSettings(t)
	got, err := st.Auth(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != AuthModeOpen {
		t.Fatalf("mode = %q, want open — a fresh install must not lock anyone out", got.Mode)
	}
	if got.UserHeader != "Remote-User" || got.GroupsHeader != "Remote-Groups" || got.GroupsSeparator != "," {
		t.Fatalf("header defaults = %+v", got)
	}
	if got.TrustedProxies == nil || len(got.TrustedProxies) != 0 {
		t.Fatalf("trusted proxies = %v, want empty non-nil slice", got.TrustedProxies)
	}
	if got.AllowTokens {
		t.Fatal("allow_tokens defaults to false")
	}
}

func TestAuthRoundTrip(t *testing.T) {
	st, _ := newTestSettings(t)
	ctx := context.Background()
	for key, val := range map[string]any{
		KeyAuthMode:           AuthModeForward,
		KeyAuthUserHeader:     "X-Forwarded-User",
		KeyAuthGroupsHeader:   "X-Forwarded-Groups",
		KeyAuthTrustedProxies: []string{"10.0.0.0/8", "192.168.1.5/32"},
		KeyAuthAdminGroup:     "admins",
		KeyAuthAllowTokens:    true,
	} {
		if err := st.Set(ctx, key, val); err != nil {
			t.Fatal(err)
		}
	}
	got, err := st.Auth(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != AuthModeForward || got.UserHeader != "X-Forwarded-User" ||
		got.AdminGroup != "admins" || !got.AllowTokens || len(got.TrustedProxies) != 2 {
		t.Fatalf("round trip = %+v", got)
	}
}
