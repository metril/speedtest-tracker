package notify_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/metril/speedtest-tracker/internal/notify"
	"github.com/metril/speedtest-tracker/internal/settings"
)

func TestDeliverWebhookPostsJSONWithHeaders(t *testing.T) {
	var body map[string]any
	var auth, ctype string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, ctype = r.Header.Get("X-Token"), r.Header.Get("Content-Type")
		json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	ch := settings.Channel{ID: "c1", Type: "webhook", URL: srv.URL,
		Headers: map[string]string{"X-Token": "abc"}}
	m := notify.Message{Kind: "alert", Title: "Home: download below 100 Mbps",
		Body: "50.0 Mbps (limit 100.0 Mbps)", Target: "Home", TargetID: 3,
		Metric: notify.MetricDownload, Value: 50, Limit: 100, Unit: "Mbps"}
	if err := notify.Deliver(context.Background(), srv.Client(), ch, m); err != nil {
		t.Fatal(err)
	}
	if ctype != "application/json" || auth != "abc" {
		t.Fatalf("content-type %q auth %q", ctype, auth)
	}
	if body["kind"] != "alert" || body["target"] != "Home" || body["metric"] != "download" {
		t.Fatalf("payload = %v", body)
	}
}

// TestDeliverAppriseSendsToConfiguredURLs verifies end-to-end delivery
// through the embedded apprise-go library: a "json://" target talks
// plain HTTP, so it can point at an httptest.Server and prove the
// library actually posts the rendered message, with no network access
// required. ch.URL is deliberately left unset — apprise channels don't
// use it.
func TestDeliverAppriseSendsToConfiguredURLs(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&body)
	}))
	defer srv.Close()

	target := "json://" + strings.TrimPrefix(srv.URL, "http://")
	ch := settings.Channel{Type: "apprise", Tags: []string{"home"}, URLs: []string{target}}
	m := notify.Message{Kind: "recovery", Title: "Home: download recovered", Body: "120.0 Mbps"}
	if err := notify.Deliver(context.Background(), nil, ch, m); err != nil {
		t.Fatal(err)
	}
	if body["title"] != m.Title || body["message"] != m.Body || body["type"] != "success" {
		t.Fatalf("payload = %v", body)
	}
}

// TestDeliverAppriseFailureType covers the notify-type mapping for a
// MetricFailure alert (failure, not the generic alert warning).
func TestDeliverAppriseFailureType(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&body)
	}))
	defer srv.Close()

	target := "json://" + strings.TrimPrefix(srv.URL, "http://")
	ch := settings.Channel{Type: "apprise", URLs: []string{target}}
	m := notify.Message{Kind: "alert", Metric: notify.MetricFailure, Title: "Home: test failed", Body: "dial tcp: refused"}
	if err := notify.Deliver(context.Background(), nil, ch, m); err != nil {
		t.Fatal(err)
	}
	if body["type"] != "failure" {
		t.Fatalf("type = %v, want failure", body["type"])
	}
}

// TestDeliverAppriseWrapsTargetError checks that a bad target surfaces
// the library's own error (which names the failing URL) unwrapped.
func TestDeliverAppriseWrapsTargetError(t *testing.T) {
	ch := settings.Channel{Type: "apprise", URLs: []string{"json://127.0.0.1:1/x"}}
	err := notify.Deliver(context.Background(), nil, ch, notify.Message{Title: "t", Body: "b"})
	if err == nil || !strings.Contains(err.Error(), "127.0.0.1:1") {
		t.Fatalf("err = %v, want one naming the failing target", err)
	}
}

// TestRedactURL covers the redaction rule: only scheme://host survives;
// userinfo, path and query (where most apprise credentials live) all
// collapse to "/***".
func TestRedactURL(t *testing.T) {
	got := notify.RedactURL("ntfy://user:pass@host/topic?token=secret123&priority=high")
	if strings.Contains(got, "secret123") || strings.Contains(got, "pass") || strings.Contains(got, "topic") {
		t.Fatalf("RedactURL = %q, want no secret/path", got)
	}
	if got != "ntfy://host/***" {
		t.Fatalf("RedactURL = %q, want ntfy://host/***", got)
	}

	// discord://webhook_id/webhook_token puts the secret in the path;
	// it must not survive redaction.
	got = notify.RedactURL("discord://123456/webhook-token-should-not-leak")
	if strings.Contains(got, "webhook-token-should-not-leak") {
		t.Fatalf("RedactURL = %q, leaks the discord webhook token", got)
	}

	if notify.RedactURL("not a url at all: %zz") == "" {
		t.Fatal("RedactURL of unparseable input must still return something")
	}
}

// TestDeliverAppriseFailureRedactsToken is the regression case for finding
// 1: a delivery failure against a bad apprise URL carrying a token must
// not leak the token in the returned error.
func TestDeliverAppriseFailureRedactsToken(t *testing.T) {
	ch := settings.Channel{Type: "apprise", URLs: []string{"json://127.0.0.1:1/x?token=leaktoken123"}}
	err := notify.Deliver(context.Background(), nil, ch, notify.Message{Title: "t", Body: "b"})
	if err == nil {
		t.Fatal("want an error")
	}
	if strings.Contains(err.Error(), "leaktoken123") {
		t.Fatalf("err = %v, leaks the token", err)
	}
}

// TestValidateChannelRedactsToken is the regression case for finding 1:
// ValidateChannel's error for a bad apprise URL must not include the raw
// URL (or its token) verbatim.
func TestValidateChannelRedactsToken(t *testing.T) {
	ch := settings.Channel{ID: "c1", Type: "apprise",
		URLs: []string{"not-a-valid-scheme://leaktoken123"}}
	err := notify.ValidateChannel(ch)
	if err == nil {
		t.Fatal("want an error")
	}
	if strings.Contains(err.Error(), "leaktoken123") {
		t.Fatalf("err = %v, leaks the token", err)
	}
}

func TestDeliverReportsNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	defer srv.Close()
	err := notify.Deliver(context.Background(), srv.Client(),
		settings.Channel{Type: "webhook", URL: srv.URL}, notify.Message{})
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("err = %v, want one mentioning 403", err)
	}
}

func TestValidateChannel(t *testing.T) {
	for _, tc := range []struct {
		name string
		ch   settings.Channel
		want string
	}{
		{"ok webhook", settings.Channel{ID: "c1", Type: "webhook", URL: "https://hook"}, ""},
		{"bad type", settings.Channel{ID: "c1", Type: "pigeon", URL: "https://x"}, "type"},
		{"bad url", settings.Channel{ID: "c1", Type: "webhook", URL: "ftp://x"}, "url"},
		{"no id", settings.Channel{Type: "webhook", URL: "https://x"}, "id"},
		{"ok apprise", settings.Channel{ID: "c1", Type: "apprise", URLs: []string{"ntfy://host/topic"}}, ""},
		{"apprise no urls", settings.Channel{ID: "c1", Type: "apprise"}, "urls"},
		{"apprise bad url", settings.Channel{ID: "c1", Type: "apprise", URLs: []string{"not-a-valid-scheme://x"}}, "apprise url"},
	} {
		err := notify.ValidateChannel(tc.ch)
		if (tc.want == "") != (err == nil) || (err != nil && !strings.Contains(err.Error(), tc.want)) {
			t.Errorf("%s: err = %v, want mention of %q", tc.name, err, tc.want)
		}
	}
}

func TestBuildMessage(t *testing.T) {
	m := notify.BuildMessage("alert", "Home", 3, 7,
		notify.Eval{Metric: notify.MetricDownload, Breached: true, Value: 50, Limit: 100, Unit: "Mbps"},
		time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC), "")
	if m.Title != "Home: download below 100.0 Mbps" {
		t.Errorf("title = %q", m.Title)
	}
	rec := notify.BuildMessage("recovery", "Home", 3, 7,
		notify.Eval{Metric: notify.MetricPing, Value: 12, Limit: 50, Unit: "ms"}, time.Now(), "")
	if rec.Title != "Home: ping recovered" {
		t.Errorf("recovery title = %q", rec.Title)
	}
	fail := notify.BuildMessage("alert", "Home", 3, 7,
		notify.Eval{Metric: notify.MetricFailure, Breached: true}, time.Now(), "dial tcp: refused")
	if !strings.Contains(fail.Body, "dial tcp: refused") {
		t.Errorf("failure body = %q, want the engine error", fail.Body)
	}
}
