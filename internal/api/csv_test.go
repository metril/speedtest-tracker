package api

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/metril/speedtest-tracker/internal/store"
)

func TestResultsCSVStreamsFilteredRows(t *testing.T) {
	h, db, _ := newTestAPI(t)
	tid, _ := seedResults(t, db, 3)
	insertFailed(t, db, tid, "2026-09-13T09:00:00.000Z")

	rec := do(t, h, http.MethodGet, "/api/v1/results.csv?status=ok", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("Content-Type = %q", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "attachment; filename=") {
		t.Errorf("Content-Disposition = %q", cd)
	}
	rows, err := csv.NewReader(rec.Body).ReadAll()
	if err != nil {
		t.Fatalf("parse csv: %v", err)
	}
	if len(rows) != 4 { // header + 3 ok results, the failure filtered out
		t.Fatalf("rows = %d: %v", len(rows), rows)
	}
	if rows[0][0] != "id" || rows[0][1] != "started_at" {
		t.Errorf("header = %v", rows[0])
	}
	if len(rows[1]) != len(rows[0]) {
		t.Errorf("row width %d != header width %d", len(rows[1]), len(rows[0]))
	}
}

func TestResultsCSVRejectsBadFilter(t *testing.T) {
	h, _, _ := newTestAPI(t)
	rec := do(t, h, http.MethodGet, "/api/v1/results.csv?from=yesterday", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body)
	}
}

// TestResultsCSVEscapesFormulaInjection guards against a malicious target
// (or tag/ISP/etc.) name turning into a live formula when the export is
// opened in a spreadsheet: cells starting with =, +, -, @, tab or CR must
// be quote-prefixed.
func TestResultsCSVEscapesFormulaInjection(t *testing.T) {
	h, db, _ := newTestAPI(t)
	ctx := context.Background()
	tid, err := db.CreateTarget(ctx, &store.Target{
		Name: "=cmd|' /C calc'!A0", Engine: "fake", Enabled: true, QueueID: 1,
		Options: json.RawMessage(`{"download_bps":7}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.InsertResult(ctx, &store.Result{
		TargetID: &tid, TargetName: "=cmd|' /C calc'!A0", Engine: "fake", Status: "ok",
		StartedAt:       "2026-09-13T10:00:00.000Z",
		OptionsSnapshot: json.RawMessage(`{}`),
	}); err != nil {
		t.Fatal(err)
	}

	rec := do(t, h, http.MethodGet, "/api/v1/results.csv", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body)
	}
	rows, err := csv.NewReader(rec.Body).ReadAll()
	if err != nil {
		t.Fatalf("parse csv: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d: %v", len(rows), rows)
	}
	if got := rows[1][3]; !strings.HasPrefix(got, "'=") {
		t.Errorf("target_name cell = %q, want a leading '=", got)
	}
}
