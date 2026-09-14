package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

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

func TestHistoryRejectsBadOffset(t *testing.T) {
	h, db, _ := newTestAPI(t)
	tid, _ := seedResults(t, db, 1)
	for _, offset := range []string{"2", "-1"} {
		rec := do(t, h, http.MethodGet, "/api/v1/targets/"+itoa(tid)+"/history?range=24h&offset="+offset, nil)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("offset=%s: status = %d, want 400", offset, rec.Code)
		}
	}
}

// TestHistoryOffsetShiftsWindowBack covers task-10-brief: offset=1 reuses
// the same window-shift as /stats/summary (shiftWindow), returning the
// immediately preceding period of equal length instead of the current one.
func TestHistoryOffsetShiftsWindowBack(t *testing.T) {
	h, db, _ := newTestAPI(t)
	tid, _ := seedResults(t, db, 1) // fixed historical timestamps, outside either window below
	if _, err := db.InsertResult(context.Background(), &store.Result{
		TargetID: &tid, TargetName: "home", Engine: "fake", Status: "ok",
		StartedAt:       time.Now().UTC().Add(-30 * time.Hour).Format("2006-01-02T15:04:05.000Z"),
		OptionsSnapshot: json.RawMessage(`{}`),
		DownloadBps:     5e7,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.InsertResult(context.Background(), &store.Result{
		TargetID: &tid, TargetName: "home", Engine: "fake", Status: "ok",
		StartedAt:       time.Now().UTC().Add(-time.Hour).Format("2006-01-02T15:04:05.000Z"),
		OptionsSnapshot: json.RawMessage(`{}`),
		DownloadBps:     8e7,
	}); err != nil {
		t.Fatal(err)
	}

	cur := do(t, h, http.MethodGet, "/api/v1/targets/"+itoa(tid)+"/history?range=24h&offset=0", nil)
	prev := do(t, h, http.MethodGet, "/api/v1/targets/"+itoa(tid)+"/history?range=24h&offset=1", nil)
	if cur.Code != http.StatusOK || prev.Code != http.StatusOK {
		t.Fatalf("status cur=%d prev=%d", cur.Code, prev.Code)
	}

	var curBody, prevBody struct {
		From, To string
		Points   []store.HistoryPoint `json:"points"`
	}
	json.NewDecoder(cur.Body).Decode(&curBody)
	json.NewDecoder(prev.Body).Decode(&prevBody)

	if curBody.From == prevBody.From || curBody.To == prevBody.To {
		t.Fatalf("offset=1 window did not shift: cur=%+v prev=%+v", curBody, prevBody)
	}
	// The -1h result falls in the current (offset=0) window; the -30h
	// result falls in the previous (offset=1) window.
	curTotal, prevTotal := 0, 0
	for _, p := range curBody.Points {
		curTotal += p.Count
	}
	for _, p := range prevBody.Points {
		prevTotal += p.Count
	}
	if curTotal == 0 {
		t.Errorf("offset=0 window has no points, want the -1h result")
	}
	if prevTotal == 0 {
		t.Errorf("offset=1 window has no points, want the -30h result")
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

func TestHistoryDefaultRangeIs24h(t *testing.T) {
	h, db, _ := newTestAPI(t)
	tid, _ := seedResults(t, db, 1)

	rec := do(t, h, http.MethodGet, "/api/v1/targets/"+itoa(tid)+"/history", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body)
	}
	var body struct{ From, To string }
	json.NewDecoder(rec.Body).Decode(&body)
	assertSpanAround(t, body.From, body.To, 24*time.Hour)
}

func TestOutagesDefaultRangeIsSevenDays(t *testing.T) {
	h, _, _ := newTestAPI(t)

	rec := do(t, h, http.MethodGet, "/api/v1/outages", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body)
	}
	var body struct{ From, To string }
	json.NewDecoder(rec.Body).Decode(&body)
	assertSpanAround(t, body.From, body.To, 7*24*time.Hour)
}

// assertSpanAround checks the from/to pair (dbTimeFormat strings) spans want,
// within a minute of slack for test execution time.
func assertSpanAround(t *testing.T, from, to string, want time.Duration) {
	t.Helper()
	f, err1 := time.Parse(dbTimeFormat, from)
	tt, err2 := time.Parse(dbTimeFormat, to)
	if err1 != nil || err2 != nil {
		t.Fatalf("parse from/to: %v / %v (from=%q to=%q)", err1, err2, from, to)
	}
	got := tt.Sub(f)
	if diff := got - want; diff < -time.Minute || diff > time.Minute {
		t.Errorf("span = %v, want ~%v", got, want)
	}
}

func TestHistoryLoneFromIsBadRequest(t *testing.T) {
	h, db, _ := newTestAPI(t)
	tid, _ := seedResults(t, db, 1)
	rec := do(t, h, http.MethodGet, "/api/v1/targets/"+itoa(tid)+"/history?from=2026-09-01T00:00:00Z", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body)
	}
}

func TestOutagesLoneToIsBadRequest(t *testing.T) {
	h, _, _ := newTestAPI(t)
	rec := do(t, h, http.MethodGet, "/api/v1/outages?to=2026-09-01T00:00:00Z", nil)
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

func TestHistoryRejectsExplicitSpanOver90Days(t *testing.T) {
	h, db, _ := newTestAPI(t)
	tid, _ := seedResults(t, db, 1)
	rec := do(t, h, http.MethodGet,
		"/api/v1/targets/"+itoa(tid)+"/history?from=2026-01-01T00:00:00Z&to=2026-06-01T00:00:00Z", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body)
	}
}

func TestOutagesRejectsExplicitSpanOver90Days(t *testing.T) {
	h, _, _ := newTestAPI(t)
	rec := do(t, h, http.MethodGet,
		"/api/v1/outages?from=2026-01-01T00:00:00Z&to=2026-06-01T00:00:00Z", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body)
	}
}

func TestHistoryAcceptsExplicitSpanUnder90Days(t *testing.T) {
	h, db, _ := newTestAPI(t)
	tid, _ := seedResults(t, db, 1)
	rec := do(t, h, http.MethodGet,
		"/api/v1/targets/"+itoa(tid)+"/history?from=2026-01-01T00:00:00Z&to=2026-03-01T00:00:00Z", nil)
	if rec.Code != http.StatusOK {
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
