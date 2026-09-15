package vlpush_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/metril/speedtest-tracker/internal/settings"
	"github.com/metril/speedtest-tracker/internal/vlpush"
)

func TestHandlerMirrorsToNextAndShipsBatch(t *testing.T) {
	bodies := make(chan string, 4)
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		b, _ := io.ReadAll(r.Body)
		bodies <- string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	var stdout bytes.Buffer
	h := vlpush.New(vlpush.Config{
		Next: slog.NewJSONHandler(&stdout, nil), App: "speedtest-tracker",
		BatchSize: 2, FlushInterval: 20 * time.Millisecond,
	})
	h.Configure(true, srv.URL, settings.ExportAuth{Type: settings.ExportAuthCustom, HeaderName: "Authorization", HeaderValue: "Bearer tok"}, map[string]string{"host": "pi4"})
	h.Start()
	defer h.Close(context.Background())

	log := slog.New(h)
	log.Info("hello", "target", "wan")
	log.Warn("uh oh")

	select {
	case body := <-bodies:
		first := strings.SplitN(strings.TrimSpace(body), "\n", 2)[0]
		var line map[string]any
		if err := json.Unmarshal([]byte(first), &line); err != nil {
			t.Fatalf("not a JSON line: %q", first)
		}
		if line["_msg"] != "hello" || line["level"] != "INFO" ||
			line["target"] != "wan" || line["app"] != "speedtest-tracker" || line["host"] != "pi4" {
			t.Fatalf("line = %v", line)
		}
		if _, ok := line["_time"]; !ok {
			t.Fatalf("_time missing: %v", line)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no batch shipped")
	}
	if !strings.Contains(gotQuery, "_stream_fields=") || !strings.Contains(gotQuery, "_msg_field=_msg") ||
		!strings.Contains(gotQuery, "_time_field=_time") {
		t.Fatalf("query = %q", gotQuery)
	}
	if !strings.Contains(stdout.String(), `"msg":"hello"`) {
		t.Fatalf("stdout mirror missing: %s", stdout.String())
	}
}

func TestHandlerAppliesExportAuth(t *testing.T) {
	for name, tc := range map[string]struct {
		auth       settings.ExportAuth
		wantAuth   string
		customHdr  string
		wantCustom string
	}{
		"none":   {auth: settings.ExportAuth{}},
		"basic":  {auth: settings.ExportAuth{Type: settings.ExportAuthBasic, Username: "u", Password: "p"}, wantAuth: "Basic dTpw"},
		"bearer": {auth: settings.ExportAuth{Type: settings.ExportAuthBearer, Token: "tok"}, wantAuth: "Bearer tok"},
		"custom": {auth: settings.ExportAuth{Type: settings.ExportAuthCustom, HeaderName: "X-Api-Key", HeaderValue: "secret"}, customHdr: "X-Api-Key", wantCustom: "secret"},
	} {
		t.Run(name, func(t *testing.T) {
			var gotAuth, gotCustom string
			hit := make(chan struct{}, 1)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotAuth = r.Header.Get("Authorization")
				if tc.customHdr != "" {
					gotCustom = r.Header.Get(tc.customHdr)
				}
				hit <- struct{}{}
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			h := vlpush.New(vlpush.Config{Next: slog.NewJSONHandler(io.Discard, nil),
				BatchSize: 1, FlushInterval: 10 * time.Millisecond})
			h.Configure(true, srv.URL, tc.auth, nil)
			h.Start()
			defer h.Close(context.Background())

			slog.New(h).Info("hello")
			select {
			case <-hit:
			case <-time.After(2 * time.Second):
				t.Fatal("no flush")
			}
			if gotAuth != tc.wantAuth {
				t.Fatalf("Authorization = %q, want %q", gotAuth, tc.wantAuth)
			}
			if tc.customHdr != "" && gotCustom != tc.wantCustom {
				t.Fatalf("%s = %q, want %q", tc.customHdr, gotCustom, tc.wantCustom)
			}
		})
	}
}

func TestHandlerDisabledStillLogsToStdout(t *testing.T) {
	var stdout bytes.Buffer
	h := vlpush.New(vlpush.Config{Next: slog.NewJSONHandler(&stdout, nil)})
	h.Start()
	defer h.Close(context.Background())
	slog.New(h).Info("local only")
	if !strings.Contains(stdout.String(), "local only") {
		t.Fatal("stdout mirror must work with shipping disabled")
	}
}

func TestHandlerDropsWhenQueueFullAndNeverBlocks(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-block }))
	defer srv.Close()
	defer close(block)
	h := vlpush.New(vlpush.Config{Next: slog.NewJSONHandler(io.Discard, nil),
		QueueSize: 4, BatchSize: 1, FlushInterval: time.Millisecond})
	h.Configure(true, srv.URL, settings.ExportAuth{}, nil)
	h.Start()
	defer h.Close(context.Background())
	log := slog.New(h)
	done := make(chan struct{})
	go func() {
		for i := 0; i < 5000; i++ {
			log.Info("flood", "i", i)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("logging blocked on a stalled VictoriaLogs")
	}
	if h.Dropped() == 0 {
		t.Fatal("expected drops with a stalled endpoint and a size-4 queue")
	}
}

// TestCloseFlushesBufferedLinesBeforeReturning is the regression case for
// the finding that Close cancelled reqCtx at the same moment it closed
// stop, so the final drainAndFlush's POST always failed instantly and every
// buffered line was dropped instead of shipped.
func TestCloseFlushesBufferedLinesBeforeReturning(t *testing.T) {
	bodies := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies <- string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// A large BatchSize and FlushInterval mean the line is still sitting in
	// the queue, unflushed, when Close is called.
	h := vlpush.New(vlpush.Config{Next: slog.NewJSONHandler(io.Discard, nil),
		BatchSize: 1000, FlushInterval: time.Hour})
	h.Configure(true, srv.URL, settings.ExportAuth{}, nil)
	h.Start()
	slog.New(h).Info("buffered at shutdown")

	if err := h.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case body := <-bodies:
		if !strings.Contains(body, "buffered at shutdown") {
			t.Fatalf("flushed body missing the line: %s", body)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not flush the buffered line")
	}
	if h.Dropped() != 0 {
		t.Fatalf("Dropped() = %d, want 0", h.Dropped())
	}
}

func TestWithAttrsAndGroupAreCarried(t *testing.T) {
	bodies := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies <- string(b)
	}))
	defer srv.Close()
	h := vlpush.New(vlpush.Config{Next: slog.NewJSONHandler(io.Discard, nil),
		BatchSize: 1, FlushInterval: 10 * time.Millisecond})
	h.Configure(true, srv.URL, settings.ExportAuth{}, nil)
	h.Start()
	defer h.Close(context.Background())
	slog.New(h).With("component", "runner").Info("started")
	select {
	case body := <-bodies:
		if !strings.Contains(body, `"component":"runner"`) {
			t.Fatalf("WithAttrs attrs lost: %s", body)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no batch")
	}
}
