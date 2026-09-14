package api

import (
	"encoding/json"
	"net/http"
	"testing"

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
