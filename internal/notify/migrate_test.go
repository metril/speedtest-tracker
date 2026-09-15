package notify_test

import (
	"bytes"
	"log/slog"
	"reflect"
	"strings"
	"testing"

	"github.com/metril/speedtest-tracker/internal/notify"
	"github.com/metril/speedtest-tracker/internal/settings"
)

func TestMigrateNtfyChannelsRewritesToApprise(t *testing.T) {
	in := []settings.Channel{
		{ID: "c1", Type: "ntfy", Name: "phone", Enabled: true,
			URL: "https://ntfy.example.com/speedtest", Token: "tk_1", Priority: "high", Tags: []string{"warning"}},
		{ID: "c2", Type: "webhook", Name: "hook", Enabled: true, URL: "https://hook"},
	}
	out, changed := notify.MigrateNtfyChannels(in, slog.Default())
	if !changed {
		t.Fatal("changed = false, want true")
	}
	if len(out) != 2 {
		t.Fatalf("len(out) = %d, want 2", len(out))
	}

	migrated := out[0]
	if migrated.Type != "apprise" {
		t.Fatalf("type = %q, want apprise", migrated.Type)
	}
	if migrated.URL != "" || migrated.Token != "" || migrated.Priority != "" {
		t.Fatalf("legacy fields not cleared: %+v", migrated)
	}
	if !migrated.Enabled {
		t.Fatal("channel disabled, want still enabled")
	}
	if len(migrated.URLs) != 1 {
		t.Fatalf("urls = %v, want one", migrated.URLs)
	}
	got := migrated.URLs[0]
	if !strings.HasPrefix(got, "ntfys://tk_1@ntfy.example.com/speedtest?") {
		t.Fatalf("url = %q, want ntfys://tk_1@ntfy.example.com/speedtest?...", got)
	}
	if !strings.Contains(got, "priority=high") || !strings.Contains(got, "tags=warning") {
		t.Fatalf("url = %q, want priority and tags carried over", got)
	}

	// Untouched: the non-ntfy channel passes through unchanged.
	if !reflect.DeepEqual(out[1], in[1]) {
		t.Fatalf("webhook channel changed: %+v", out[1])
	}
}

func TestMigrateNtfyChannelsHTTPStaysPlainNtfy(t *testing.T) {
	out, changed := notify.MigrateNtfyChannels([]settings.Channel{
		{ID: "c1", Type: "ntfy", Enabled: true, URL: "http://ntfy.internal/topic"},
	}, nil)
	if !changed {
		t.Fatal("changed = false, want true")
	}
	if !strings.HasPrefix(out[0].URLs[0], "ntfy://ntfy.internal/topic") {
		t.Fatalf("url = %q, want ntfy:// (plain) scheme", out[0].URLs[0])
	}
}

func TestMigrateNtfyChannelsNoOpWithoutNtfy(t *testing.T) {
	in := []settings.Channel{{ID: "c1", Type: "webhook", URL: "https://hook"}}
	out, changed := notify.MigrateNtfyChannels(in, nil)
	if changed {
		t.Fatal("changed = true, want false")
	}
	if len(out) != 1 || !reflect.DeepEqual(out[0], in[0]) {
		t.Fatalf("out = %+v, want unchanged", out)
	}
}

func TestMigrateNtfyChannelsDisablesOnUnparseableURL(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	out, changed := notify.MigrateNtfyChannels([]settings.Channel{
		{ID: "c1", Type: "ntfy", Enabled: true, URL: "://not a url"},
	}, logger)
	if !changed {
		t.Fatal("changed = false, want true")
	}
	got := out[0]
	if got.Type != "apprise" {
		t.Fatalf("type = %q, want apprise", got.Type)
	}
	if got.Enabled {
		t.Fatal("channel still enabled, want disabled")
	}
	if len(got.URLs) != 0 {
		t.Fatalf("urls = %v, want none", got.URLs)
	}
	if !strings.Contains(buf.String(), "c1") {
		t.Fatalf("log = %q, want it to mention the channel", buf.String())
	}
}

func TestMigrateNtfyChannelsDisablesOnMissingHost(t *testing.T) {
	out, changed := notify.MigrateNtfyChannels([]settings.Channel{
		{ID: "c1", Type: "ntfy", Enabled: true, URL: "not-a-url-at-all"},
	}, nil)
	if !changed {
		t.Fatal("changed = false, want true")
	}
	if out[0].Enabled {
		t.Fatal("channel still enabled, want disabled")
	}
}
