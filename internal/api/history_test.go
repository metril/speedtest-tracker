package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/metril/speedtest-tracker/internal/store"
)

func TestHistoryRangeReturnsBucketedPoints(t *testing.T) {
	h, db, _ := newTestAPI(t)
	tid, _ := seedResults(t, db, 3)

	rec := do(t, h, http.MethodGet, "/api/v1/targets/"+itoa(tid)+"/history?range=30d", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body)
	}
	var body struct {
		TargetID      int64                `json:"target_id"`
		BucketSeconds int                  `json:"bucket_seconds"`
		Points        []store.HistoryPoint `json:"points"`
	}
	json.NewDecoder(rec.Body).Decode(&body)
	if body.TargetID != tid {
		t.Errorf("target_id = %d", body.TargetID)
	}
	if body.BucketSeconds < 60 {
		t.Errorf("bucket_seconds = %d", body.BucketSeconds)
	}
	if len(body.Points) > 500 {
		t.Errorf("points = %d, want <= 500", len(body.Points))
	}
}

func TestHistoryRejectsUnknownRange(t *testing.T) {
	h, db, _ := newTestAPI(t)
	tid, _ := seedResults(t, db, 1)
	rec := do(t, h, http.MethodGet, "/api/v1/targets/"+itoa(tid)+"/history?range=99y", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body)
	}
}

func TestHistoryUnknownTargetIs404(t *testing.T) {
	h, _, _ := newTestAPI(t)
	rec := do(t, h, http.MethodGet, "/api/v1/targets/424242/history?range=24h", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body)
	}
}

func TestOutagesListsIncidents(t *testing.T) {
	h, db, _ := newTestAPI(t)
	tid, _ := seedResults(t, db, 1)
	insertFailed(t, db, tid, "2026-09-13T10:00:00.000Z")

	rec := do(t, h, http.MethodGet, "/api/v1/outages?from=2026-09-01T00:00:00Z&to=2026-10-01T00:00:00Z", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body)
	}
	var body struct {
		Incidents []store.Incident `json:"incidents"`
	}
	json.NewDecoder(rec.Body).Decode(&body)
	if len(body.Incidents) != 1 || body.Incidents[0].Kind != "result" {
		t.Fatalf("incidents = %+v", body.Incidents)
	}
}
