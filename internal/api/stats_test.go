package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/metril/speedtest-tracker/internal/store"
)

func TestStatsSummaryReturnsTargetsAndCacheHeader(t *testing.T) {
	h, db, _ := newTestAPI(t)
	seedResults(t, db, 2)

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
