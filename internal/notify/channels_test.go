package notify_test

import (
	"context"
	"encoding/json"
	"io"
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

func TestDeliverNtfyUsesHeadersAndToken(t *testing.T) {
	var h http.Header
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h = r.Header.Clone()
		body, _ = io.ReadAll(r.Body)
	}))
	defer srv.Close()

	ch := settings.Channel{Type: "ntfy", URL: srv.URL, Token: "tk_1",
		Priority: "high", Tags: []string{"warning", "satellite"}}
	m := notify.Message{Kind: "alert", Title: "Home: ping above 50 ms", Body: "80.0 ms (limit 50.0 ms)"}
	if err := notify.Deliver(context.Background(), srv.Client(), ch, m); err != nil {
		t.Fatal(err)
	}
	if h.Get("Title") != m.Title || h.Get("Priority") != "high" ||
		h.Get("Tags") != "warning,satellite" || h.Get("Authorization") != "Bearer tk_1" {
		t.Fatalf("headers = %v", h)
	}
	if string(body) != m.Body {
		t.Fatalf("body = %q, want %q", body, m.Body)
	}
}

func TestDeliverAppriseSendsTagsAndURLs(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&body)
	}))
	defer srv.Close()

	ch := settings.Channel{Type: "apprise", URL: srv.URL + "/notify",
		Tags: []string{"home"}, URLs: []string{"mailto://a:b@example.com"}}
	m := notify.Message{Kind: "recovery", Title: "Home: download recovered", Body: "120.0 Mbps"}
	if err := notify.Deliver(context.Background(), srv.Client(), ch, m); err != nil {
		t.Fatal(err)
	}
	if body["title"] != m.Title || body["body"] != m.Body || body["type"] != "success" || body["tag"] != "home" {
		t.Fatalf("payload = %v", body)
	}
	if urls, _ := body["urls"].([]any); len(urls) != 1 {
		t.Fatalf("urls = %v", body["urls"])
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
		{"ok", settings.Channel{ID: "c1", Type: "ntfy", URL: "https://ntfy.sh/x"}, ""},
		{"bad type", settings.Channel{ID: "c1", Type: "pigeon", URL: "https://x"}, "type"},
		{"bad url", settings.Channel{ID: "c1", Type: "ntfy", URL: "ftp://x"}, "url"},
		{"no id", settings.Channel{Type: "ntfy", URL: "https://x"}, "id"},
		{"bad priority", settings.Channel{ID: "c", Type: "ntfy", URL: "https://x", Priority: "loudest"}, "priority"},
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
