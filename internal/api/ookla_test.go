package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/metril/speedtest-tracker/internal/engine/ookla"
	"github.com/metril/speedtest-tracker/internal/ooklaweb"
)

// stubSearcher is a scripted ServerSearcher.
type stubSearcher struct {
	result ooklaweb.SearchResult
	err    error

	calls      int
	gotQ       string
	gotCountry string
	gotLimit   int
}

func (s *stubSearcher) Search(_ context.Context, req ooklaweb.SearchRequest) (ooklaweb.SearchResult, error) {
	s.calls++
	s.gotQ = req.Q
	s.gotCountry = req.Country
	s.gotLimit = req.Limit
	if s.err != nil {
		return ooklaweb.SearchResult{}, s.err
	}
	return s.result, nil
}

// ooklaListResponse mirrors GET /ookla/servers' response shape.
type ooklaListResponse struct {
	Servers []ookla.Server `json:"servers"`
	Near    string         `json:"near"`
}

func decodeOoklaList(t *testing.T, body *bytes.Buffer) ooklaListResponse {
	t.Helper()
	var got ooklaListResponse
	if err := json.NewDecoder(body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestOoklaServerSearchMergesNewRemoteHits(t *testing.T) {
	search := &stubSearcher{result: ooklaweb.SearchResult{Servers: []ookla.Server{
		{ID: "101", Name: "Comcast", Location: "Denver, CO", Country: "United States",
			Sponsor: "Comcast", Host: "denver.example:8080", Lat: 39.7, Lon: -104.9, DistanceKm: 1.1},
	}}}
	h, _, _ := newTestAPIWith(t, func(d *Deps) { d.OoklaSearch = search })

	rec := do(t, h, http.MethodGet, "/api/v1/ookla/servers?q=denver", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	got := decodeOoklaList(t, rec.Body)
	if len(got.Servers) != 1 || got.Servers[0].ID != "101" || got.Servers[0].DistanceKm != 1.1 || got.Servers[0].Sponsor != "Comcast" {
		t.Fatalf("got = %+v", got)
	}
	if search.calls != 1 || search.gotQ != "denver" {
		t.Errorf("search called %d times with q=%q, want 1 call with q=denver", search.calls, search.gotQ)
	}
}

func TestOoklaServerSearchDedupesRemoteAgainstLocalMatches(t *testing.T) {
	search := &stubSearcher{result: ooklaweb.SearchResult{Servers: []ookla.Server{
		{ID: "1", Name: "Frankfurt Fiber", Location: "Frankfurt", Country: "Germany", Host: "fra.example:8080"}, // dup of local id 1
		{ID: "55", Name: "Vodafone", Location: "Frankfurt", Country: "Germany", Host: "fra2.example:8080"},
	}}}
	h, _, _ := newTestAPIWith(t, func(d *Deps) { d.OoklaSearch = search })

	rec := do(t, h, http.MethodGet, "/api/v1/ookla/servers?q=frank", nil)
	got := decodeOoklaList(t, rec.Body)
	if len(got.Servers) != 2 {
		t.Fatalf("got %d servers, want 2 (local id=1 kept once, remote id=55 added): %+v", len(got.Servers), got)
	}
	ids := map[string]bool{}
	for _, s := range got.Servers {
		ids[s.ID] = true
	}
	if !ids["1"] || !ids["55"] {
		t.Errorf("got ids = %+v, want {1,55}", ids)
	}
}

func TestOoklaServerSearchFallsBackOnRemoteError(t *testing.T) {
	var buf bytes.Buffer
	search := &stubSearcher{err: errors.New("upstream unreachable")}
	h, _, _ := newTestAPIWith(t, func(d *Deps) {
		d.OoklaSearch = search
		d.Logger = slog.New(slog.NewTextHandler(&buf, nil))
	})

	rec := do(t, h, http.MethodGet, "/api/v1/ookla/servers?q=frank", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (local list still served)", rec.Code)
	}
	got := decodeOoklaList(t, rec.Body)
	if len(got.Servers) != 1 || got.Servers[0].ID != "1" {
		t.Fatalf("got = %+v, want local-only match", got)
	}
	if !strings.Contains(buf.String(), "level=WARN") || !strings.Contains(buf.String(), "upstream unreachable") {
		t.Errorf("log output = %q, want a WARN mentioning the remote error", buf.String())
	}
}

func TestOoklaServerSearchSkipsRemoteWithoutQuery(t *testing.T) {
	search := &stubSearcher{result: ooklaweb.SearchResult{Servers: []ookla.Server{{ID: "999", Name: "Should not appear"}}}}
	h, _, _ := newTestAPIWith(t, func(d *Deps) { d.OoklaSearch = search })

	rec := do(t, h, http.MethodGet, "/api/v1/ookla/servers", nil)
	got := decodeOoklaList(t, rec.Body)
	if len(got.Servers) != 2 {
		t.Fatalf("got %d servers, want 2 (local only)", len(got.Servers))
	}
	if search.calls != 0 {
		t.Errorf("search called %d times, want 0 for an empty query", search.calls)
	}
}

func TestOoklaServerSearchRespectsLimit(t *testing.T) {
	search := &stubSearcher{result: ooklaweb.SearchResult{Servers: []ookla.Server{
		{ID: "101", Name: "A"}, {ID: "102", Name: "B"}, {ID: "103", Name: "C"},
	}}}
	h, _, _ := newTestAPIWith(t, func(d *Deps) { d.OoklaSearch = search })

	rec := do(t, h, http.MethodGet, "/api/v1/ookla/servers?q=x&limit=2", nil)
	got := decodeOoklaList(t, rec.Body)
	if len(got.Servers) != 2 {
		t.Fatalf("got %d servers, want 2 (limit)", len(got.Servers))
	}
	if search.gotLimit != 2 {
		t.Errorf("search called with limit=%d, want 2", search.gotLimit)
	}
}

// errServerList always fails, simulating e.g. the speedtest binary missing:
// exec: "speedtest": executable file not found in $PATH.
type errServerList struct{ err error }

func (e errServerList) Servers(context.Context) ([]ookla.Server, error) { return nil, e.err }

func TestOoklaServerSearchCapsLimit(t *testing.T) {
	search := &stubSearcher{result: ooklaweb.SearchResult{Servers: []ookla.Server{{ID: "101", Name: "A"}}}}
	h, _, _ := newTestAPIWith(t, func(d *Deps) { d.OoklaSearch = search })

	rec := do(t, h, http.MethodGet, "/api/v1/ookla/servers?q=x&limit=5000", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if search.gotLimit != maxOoklaSearchLimit {
		t.Errorf("search called with limit=%d, want capped %d", search.gotLimit, maxOoklaSearchLimit)
	}
}

func TestOoklaServerSearchLocalErrorFallsBackToRemote(t *testing.T) {
	search := &stubSearcher{result: ooklaweb.SearchResult{Servers: []ookla.Server{
		{ID: "101", Name: "Comcast", Location: "Denver, CO"},
	}}}
	h, _, _ := newTestAPIWith(t, func(d *Deps) {
		d.ServerList = errServerList{err: errors.New(`exec: "speedtest": executable file not found in $PATH`)}
		d.OoklaSearch = search
	})

	rec := do(t, h, http.MethodGet, "/api/v1/ookla/servers?q=denver", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (remote still served despite local failure)", rec.Code)
	}
	got := decodeOoklaList(t, rec.Body)
	if len(got.Servers) != 1 || got.Servers[0].ID != "101" {
		t.Fatalf("got = %+v, want remote-only hit", got)
	}
}

func TestOoklaServerSearchBothSourcesFailIs502(t *testing.T) {
	search := &stubSearcher{err: errors.New("upstream unreachable")}
	h, _, _ := newTestAPIWith(t, func(d *Deps) {
		d.ServerList = errServerList{err: errors.New("local list unavailable")}
		d.OoklaSearch = search
	})

	rec := do(t, h, http.MethodGet, "/api/v1/ookla/servers?q=denver", nil)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502 (both sources failed)", rec.Code)
	}
}

func TestOoklaServerSearchLocalErrorWithNoSearcherIs502(t *testing.T) {
	h, _, _ := newTestAPIWith(t, func(d *Deps) {
		d.ServerList = errServerList{err: errors.New("local list unavailable")}
		d.OoklaSearch = nil
	})

	rec := do(t, h, http.MethodGet, "/api/v1/ookla/servers?q=denver", nil)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502 (local failed, no remote to fall back on)", rec.Code)
	}
}

// denyLimiter always denies, simulating an exhausted rate limiter.
type denyLimiter struct{ calls int }

func (l *denyLimiter) Allow() bool { l.calls++; return false }

// TestOoklaServerSearchRateLimiterDeniesFallsBackToLocal covers
// task-1-brief item 1: when OoklaLimiter denies, the remote search must be
// skipped entirely (never called), local results are still served, the
// response is 200 (never an error), and the denial is logged at debug.
func TestOoklaServerSearchRateLimiterDeniesFallsBackToLocal(t *testing.T) {
	var buf bytes.Buffer
	search := &stubSearcher{result: ooklaweb.SearchResult{Servers: []ookla.Server{{ID: "999", Name: "Should not appear"}}}}
	limiter := &denyLimiter{}
	h, _, _ := newTestAPIWith(t, func(d *Deps) {
		d.OoklaSearch = search
		d.OoklaLimiter = limiter
		d.Logger = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	})

	rec := do(t, h, http.MethodGet, "/api/v1/ookla/servers?q=frank", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (limiter denial must not error)", rec.Code)
	}
	got := decodeOoklaList(t, rec.Body)
	if len(got.Servers) != 1 || got.Servers[0].ID != "1" {
		t.Fatalf("got = %+v, want local-only match", got)
	}
	if search.calls != 0 {
		t.Errorf("remote search called %d times, want 0 when the limiter denies", search.calls)
	}
	if limiter.calls == 0 {
		t.Error("limiter was never consulted")
	}
	if !strings.Contains(buf.String(), "level=DEBUG") || !strings.Contains(buf.String(), "rate limited") {
		t.Errorf("log output = %q, want a DEBUG line about the rate limit", buf.String())
	}
}

// allowLimiter always allows, recording how many times it was consulted.
type allowLimiter struct{ calls int }

func (l *allowLimiter) Allow() bool { l.calls++; return true }

func TestOoklaServerSearchLimiterAllowsRemoteSearch(t *testing.T) {
	search := &stubSearcher{result: ooklaweb.SearchResult{Servers: []ookla.Server{{ID: "101", Name: "Comcast"}}}}
	limiter := &allowLimiter{}
	h, _, _ := newTestAPIWith(t, func(d *Deps) {
		d.OoklaSearch = search
		d.OoklaLimiter = limiter
	})

	rec := do(t, h, http.MethodGet, "/api/v1/ookla/servers?q=frank", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if search.calls != 1 {
		t.Errorf("remote search called %d times, want 1 when the limiter allows", search.calls)
	}
	if limiter.calls != 1 {
		t.Errorf("limiter consulted %d times, want 1", limiter.calls)
	}
}

// TestTokenBucketLimiterBurstAndRefill covers task-1-brief item 1's exact
// shape: burst 10, refill 1/s.
func TestTokenBucketLimiterBurstAndRefill(t *testing.T) {
	now := time.Now()
	l := &tokenBucketLimiter{tokens: ooklaLimiterBurst, burst: ooklaLimiterBurst, refillPerSec: ooklaLimiterRefillPerSecond, now: func() time.Time { return now }}

	for i := 0; i < 10; i++ {
		if !l.Allow() {
			t.Fatalf("call %d denied, want the initial burst of 10 to be allowed", i)
		}
	}
	if l.Allow() {
		t.Fatal("11th call within the burst allowed, want denied")
	}

	// After 1s, exactly one more token has refilled.
	now = now.Add(1 * time.Second)
	if !l.Allow() {
		t.Fatal("call after 1s refill denied, want allowed")
	}
	if l.Allow() {
		t.Fatal("second call right after a single 1s refill allowed, want denied")
	}
}

func TestOoklaServerSearchLocalErrorWithEmptyQueryReturnsEmptyList(t *testing.T) {
	h, _, _ := newTestAPIWith(t, func(d *Deps) {
		d.ServerList = errServerList{err: errors.New("local list unavailable")}
		d.OoklaSearch = nil
	})

	rec := do(t, h, http.MethodGet, "/api/v1/ookla/servers", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (empty query never attempts remote, so this isn't a hard failure)", rec.Code)
	}
	got := decodeOoklaList(t, rec.Body)
	if len(got.Servers) != 0 {
		t.Fatalf("got = %+v, want []", got)
	}
}

func TestOoklaServerSearchPassesCountryAndNear(t *testing.T) {
	search := &stubSearcher{result: ooklaweb.SearchResult{
		Servers: []ookla.Server{{ID: "101", Name: "A"}},
		Near:    "Denver, Colorado",
	}}
	h, _, _ := newTestAPIWith(t, func(d *Deps) { d.OoklaSearch = search })

	rec := do(t, h, http.MethodGet, "/api/v1/ookla/servers?q=80202&country=US", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body)
	}
	if search.gotCountry != "us" {
		t.Errorf("gotCountry = %q, want lowercased \"us\"", search.gotCountry)
	}
	got := decodeOoklaList(t, rec.Body)
	if got.Near != "Denver, Colorado" {
		t.Errorf("near = %q, want %q", got.Near, "Denver, Colorado")
	}
}

func TestOoklaServerSearchRejectsInvalidCountry(t *testing.T) {
	search := &stubSearcher{result: ooklaweb.SearchResult{Servers: []ookla.Server{}}}
	h, _, _ := newTestAPIWith(t, func(d *Deps) { d.OoklaSearch = search })

	for _, country := range []string{"USA", "1", "u1"} {
		rec := do(t, h, http.MethodGet, "/api/v1/ookla/servers?q=x&country="+country, nil)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("country=%q: status = %d, want 400", country, rec.Code)
		}
	}
	if search.calls != 0 {
		t.Errorf("search called %d times, want 0 for a rejected country", search.calls)
	}
}
