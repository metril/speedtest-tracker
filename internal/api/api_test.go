package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/metril/speedtest-tracker/internal/sse"
)

type stubPinger struct{ err error }

func (s stubPinger) Ping(context.Context) error { return s.err }

func testHandler(pingErr error) http.Handler {
	return New(Deps{
		Pinger: stubPinger{err: pingErr},
		Logger: slog.New(slog.NewJSONHandler(io.Discard, nil)),
		UI: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("spa"))
		}),
	})
}

func TestHealthzOK(t *testing.T) {
	rec := httptest.NewRecorder()
	testHandler(nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if body := rec.Body.String(); body != `{"status":"ok"}` {
		t.Errorf("body = %q", body)
	}
}

func TestHealthzUnhealthyWhenDBDown(t *testing.T) {
	rec := httptest.NewRecorder()
	testHandler(errors.New("boom")).ServeHTTP(rec,
		httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if body := rec.Body.String(); body != `{"status":"unhealthy","error":"database unreachable"}` {
		t.Errorf("body = %q", body)
	}
}

func TestUnknownPathFallsThroughToUI(t *testing.T) {
	rec := httptest.NewRecorder()
	testHandler(nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/dashboard", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "spa" {
		t.Fatalf("status = %d body = %q, want 200 spa", rec.Code, rec.Body.String())
	}
}

func TestCompressionIsApplied(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	testHandler(nil).ServeHTTP(rec, req)
	// The body is short, so chi may skip compressing it; assert the
	// middleware negotiated at all by checking Vary is set.
	if v := rec.Header().Get("Vary"); v != "Accept-Encoding" {
		t.Errorf("Vary = %q, want Accept-Encoding", v)
	}
}

func TestRequestIDHeaderIsSet(t *testing.T) {
	rec := httptest.NewRecorder()
	testHandler(nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Header().Get("X-Request-Id") == "" {
		t.Error("X-Request-Id header missing")
	}
}

func TestSecurityHeaders(t *testing.T) {
	h := New(Deps{
		Pinger: stubPinger{},
		Logger: slog.New(slog.NewJSONHandler(io.Discard, nil)),
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	want := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	}
	for k, v := range want {
		if got := rec.Header().Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
}

// TestEventsStreamIsNeverGzipCompressed drives /api/v1/events through the
// full api.New router (including the global Compress middleware) with a
// client that advertises gzip, and asserts the SSE stream is delivered
// uncompressed and an event is received intact: the compress middleware
// only compresses content types on its allow-list, and text/event-stream
// is not one of them, so this must hold regardless of Accept-Encoding.
func TestEventsStreamIsNeverGzipCompressed(t *testing.T) {
	hub := sse.NewHub()
	srv := httptest.NewServer(New(Deps{
		Pinger: stubPinger{},
		Logger: slog.New(slog.NewJSONHandler(io.Discard, nil)),
		Hub:    hub,
	}))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := http.DefaultTransport.RoundTrip(req) // bypass DefaultClient's transparent gzip handling
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if ce := resp.Header.Get("Content-Encoding"); ce != "" {
		t.Errorf("Content-Encoding = %q, want unset (SSE must never be compressed)", ce)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q", ct)
	}

	deadline := time.Now().Add(2 * time.Second)
	for hub.Subscribers() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	hub.Publish(hub.Marshal(sse.EventRun, map[string]any{"run_id": 1, "status": "queued", "error": ""}))

	sc := bufio.NewScanner(resp.Body)
	var gotEvent, gotData bool
	for sc.Scan() {
		line := sc.Text()
		if line == "event: run" {
			gotEvent = true
		}
		if strings.Contains(line, `"run_id":1`) {
			gotData = true
			break
		}
	}
	if !gotEvent || !gotData {
		t.Errorf("event=%v data=%v (stream must carry plain, uncompressed SSE text)", gotEvent, gotData)
	}
}

// TestResultsCSVRouteDoesNotShadowResultByID verifies that pulling
// /api/v1/results.csv out of the v1 timeout group (so it can use its own,
// longer timeout) into a root-level route still leaves it served, and that
// it does not collide with /api/v1/results/{id} in the process.
func TestResultsCSVRouteDoesNotShadowResultByID(t *testing.T) {
	h, db, _ := newTestAPI(t)
	_, ids := seedResults(t, db, 1)

	rec := do(t, h, http.MethodGet, "/api/v1/results.csv", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("results.csv status = %d body=%s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("results.csv Content-Type = %q", ct)
	}

	rec = do(t, h, http.MethodGet, "/api/v1/results/"+itoa(ids[0]), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("results/{id} status = %d body=%s", rec.Code, rec.Body)
	}
}

func TestWriteErrorEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusBadRequest, "invalid_request", "name is required")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content type = %q", ct)
	}
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error.Code != "invalid_request" || body.Error.Message != "name is required" {
		t.Errorf("body = %+v", body.Error)
	}
}
