package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
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
