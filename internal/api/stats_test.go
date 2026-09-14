package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/metril/speedtest-tracker/internal/store"
)

// countingMetrics implements CacheMetrics for tests.
type countingMetrics struct {
	hits   *atomic.Int32
	misses *atomic.Int32
}

func (c countingMetrics) SummaryCacheHit()  { c.hits.Add(1) }
func (c countingMetrics) SummaryCacheMiss() { c.misses.Add(1) }

func TestSummaryCacheCountedInMetrics(t *testing.T) {
	var hits, misses atomic.Int32
	h, db, _ := newTestAPIWith(t, func(d *Deps) { d.Metrics = countingMetrics{&hits, &misses} })
	seedResults(t, db, 1)
	do(t, h, http.MethodGet, "/api/v1/stats/summary?range=24h", nil)
	do(t, h, http.MethodGet, "/api/v1/stats/summary?range=24h", nil)
	if misses.Load() != 1 || hits.Load() != 1 {
		t.Fatalf("hits=%d misses=%d, want 1/1", hits.Load(), misses.Load())
	}
}

func TestStatsSummaryReturnsTargetsAndCacheHeader(t *testing.T) {
	h, db, _ := newTestAPI(t)
	tid, _ := seedResults(t, db, 2)
	// seedResults uses fixed historical timestamps; the 24h window needs a
	// result relative to the real clock.
	if _, err := db.InsertResult(context.Background(), &store.Result{
		TargetID: &tid, TargetName: "home", Engine: "fake", Status: "ok",
		StartedAt:       time.Now().UTC().Add(-time.Minute).Format("2006-01-02T15:04:05.000Z"),
		OptionsSnapshot: json.RawMessage(`{}`),
		DownloadBps:     1e8,
	}); err != nil {
		t.Fatal(err)
	}

	rec := do(t, h, http.MethodGet, "/api/v1/stats/summary?range=24h", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Cache-Control"); got != "max-age=30" {
		t.Errorf("Cache-Control = %q", got)
	}
	var body store.SummaryStats
	json.NewDecoder(rec.Body).Decode(&body)
	if len(body.Targets) != 1 || body.Targets[0].Count == 0 {
		t.Fatalf("summary = %+v", body)
	}
}

func TestStatsSummaryServesFromCacheWithinTTL(t *testing.T) {
	h, db, _ := newTestAPI(t)
	seedResults(t, db, 1)
	first := do(t, h, http.MethodGet, "/api/v1/stats/summary?range=24h", nil).Body.String()

	seedResults(t, db, 1) // a second target's worth of rows, invisible until the TTL expires
	second := do(t, h, http.MethodGet, "/api/v1/stats/summary?range=24h", nil).Body.String()
	if first != second {
		t.Fatal("summary was recomputed inside the 30s cache window")
	}
}

func TestStatsSummaryRejectsExplicitFromTo(t *testing.T) {
	h, _, _ := newTestAPI(t)
	rec := do(t, h, http.MethodGet, "/api/v1/stats/summary?from=1700000000&to=1700003600", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestStatsSummaryRejectsBadOffset(t *testing.T) {
	h, _, _ := newTestAPI(t)
	rec := do(t, h, http.MethodGet, "/api/v1/stats/summary?range=24h&offset=2", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// TestStatsSummaryOffsetShiftsWindowBack verifies offset=1 returns the
// immediately preceding window of equal span: a result placed just before
// the current 24h window is invisible at offset=0 but visible at offset=1,
// and the two bodies differ (different cache key).
func TestStatsSummaryOffsetShiftsWindowBack(t *testing.T) {
	h, db, _ := newTestAPI(t)
	tid, _ := seedResults(t, db, 1)
	if _, err := db.InsertResult(context.Background(), &store.Result{
		TargetID: &tid, TargetName: "home", Engine: "fake", Status: "ok",
		StartedAt:       time.Now().UTC().Add(-30 * time.Hour).Format("2006-01-02T15:04:05.000Z"),
		OptionsSnapshot: json.RawMessage(`{}`),
		DownloadBps:     5e7,
	}); err != nil {
		t.Fatal(err)
	}

	cur := do(t, h, http.MethodGet, "/api/v1/stats/summary?range=24h&offset=0", nil)
	prev := do(t, h, http.MethodGet, "/api/v1/stats/summary?range=24h&offset=1", nil)
	if cur.Code != http.StatusOK || prev.Code != http.StatusOK {
		t.Fatalf("status cur=%d prev=%d", cur.Code, prev.Code)
	}
	if cur.Body.String() == prev.Body.String() {
		t.Fatal("offset=1 returned the same body as offset=0")
	}
	var prevBody store.SummaryStats
	json.NewDecoder(prev.Body).Decode(&prevBody)
	if len(prevBody.Targets) == 0 || prevBody.Targets[0].Count == 0 {
		t.Fatalf("previous window summary = %+v, want the -30h result", prevBody)
	}
}

func TestSummaryCacheEvictsOldestBeyondCap(t *testing.T) {
	c := newSummaryCache(time.Minute)
	for i := 0; i < 100; i++ {
		c.set(fmt.Sprintf("k%d", i), []byte("x"))
	}
	if len(c.entries) > maxSummaryCacheEntries {
		t.Fatalf("cache has %d entries, want <= %d", len(c.entries), maxSummaryCacheEntries)
	}
	if _, ok := c.get("k0"); ok {
		t.Error("oldest entry k0 should have been evicted")
	}
	if _, ok := c.get("k99"); !ok {
		t.Error("newest entry k99 should still be cached")
	}
}
