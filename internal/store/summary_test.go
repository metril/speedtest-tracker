package store

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

func TestSummaryPerTargetAndOverall(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	a, _ := s.CreateTarget(ctx, &Target{Name: "home", Engine: "fake", Enabled: true, Lane: "wan"})
	b, _ := s.CreateTarget(ctx, &Target{Name: "nas", Engine: "fake", Enabled: true, Lane: "lan"})
	insertResultAt(t, s, a, "fake", "ok", "2026-09-13T10:00:00.000Z")
	insertResultAt(t, s, a, "fake", "failed", "2026-09-13T11:00:00.000Z")
	insertResultAt(t, s, b, "fake", "ok", "2026-09-13T12:00:00.000Z")

	got, err := s.Summary(ctx, "2026-09-13T00:00:00.000Z", "2026-09-14T00:00:00.000Z", SLAPlan{})
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if got.TotalResults != 3 || got.TotalFailures != 1 {
		t.Errorf("totals = %d/%d", got.TotalResults, got.TotalFailures)
	}
	if got.SuccessRate < 0.66 || got.SuccessRate > 0.67 {
		t.Errorf("success rate = %v", got.SuccessRate)
	}
	if len(got.Targets) != 2 {
		t.Fatalf("targets = %+v", got.Targets)
	}
	home := got.Targets[0]
	if home.TargetName != "home" || home.Count != 2 || home.FailCount != 1 {
		t.Errorf("home = %+v", home)
	}
	if home.Latest == nil || home.Latest.Status != "failed" {
		t.Errorf("home latest = %+v", home.Latest)
	}
	if home.AvgDownloadBps != 100e6 || home.MaxDownloadBps != 100e6 {
		t.Errorf("home download = %+v", home)
	}
}

func TestSummaryIncludesTargetWithoutResultsInWindow(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	a, _ := s.CreateTarget(ctx, &Target{Name: "cold", Engine: "fake", Enabled: true, Lane: "wan"})
	insertResultAt(t, s, a, "fake", "ok", "2026-08-01T10:00:00.000Z")

	got, err := s.Summary(ctx, "2026-09-13T00:00:00.000Z", "2026-09-14T00:00:00.000Z", SLAPlan{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Targets) != 1 || got.Targets[0].Count != 0 {
		t.Fatalf("targets = %+v", got.Targets)
	}
	if got.Targets[0].Latest == nil {
		t.Error("latest result outside the window should still be reported")
	}
}

// TestSummaryExcludesFailedNonZeroReadingFromAggregates checks that a
// failed row's own nonzero partial reading never enters avg/min/max, even
// though NULLIF(x,0) would have let a nonzero value through regardless of
// status.
func TestSummaryExcludesFailedNonZeroReadingFromAggregates(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	a, _ := s.CreateTarget(ctx, &Target{Name: "home", Engine: "fake", Enabled: true, Lane: "wan"})
	insertResultAt(t, s, a, "fake", "ok", "2026-09-13T10:00:00.000Z") // download_bps=100e6
	if _, err := s.InsertResult(ctx, &Result{
		TargetID: &a, TargetName: "home", Engine: "fake", Status: "failed",
		StartedAt: "2026-09-13T11:00:00.000Z", DurationMs: 500,
		OptionsSnapshot: json.RawMessage(`{}`),
		DownloadBps:     999e6, UploadBps: 999e6, PingMs: 999,
	}); err != nil {
		t.Fatalf("InsertResult: %v", err)
	}

	got, err := s.Summary(ctx, "2026-09-13T00:00:00.000Z", "2026-09-14T00:00:00.000Z", SLAPlan{})
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if len(got.Targets) != 1 || got.Targets[0].Count != 2 || got.Targets[0].FailCount != 1 {
		t.Fatalf("targets = %+v", got.Targets)
	}
	ts := got.Targets[0]
	if ts.AvgDownloadBps != 100e6 || ts.MinDownloadBps != 100e6 || ts.MaxDownloadBps != 100e6 {
		t.Errorf("download aggregates = %+v, want all 100e6 (999e6 failed reading excluded)", ts)
	}
	if ts.MaxPingMs != 12.5 {
		t.Errorf("MaxPingMs = %v, want 12.5 (999 failed reading excluded)", ts.MaxPingMs)
	}
}

// TestSummaryIncludesGenuineZeroFromOkRow checks an ok row's real 0 reading
// is counted rather than treated as missing.
func TestSummaryIncludesGenuineZeroFromOkRow(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	a, _ := s.CreateTarget(ctx, &Target{Name: "home", Engine: "fake", Enabled: true, Lane: "wan"})
	insertResultAt(t, s, a, "fake", "ok", "2026-09-13T10:00:00.000Z") // download_bps=100e6
	if _, err := s.InsertResult(ctx, &Result{
		TargetID: &a, TargetName: "home", Engine: "fake", Status: "ok",
		StartedAt: "2026-09-13T11:00:00.000Z", DurationMs: 500,
		OptionsSnapshot: json.RawMessage(`{}`),
		DownloadBps:     0, UploadBps: 0, PingMs: 0,
	}); err != nil {
		t.Fatalf("InsertResult: %v", err)
	}

	got, err := s.Summary(ctx, "2026-09-13T00:00:00.000Z", "2026-09-14T00:00:00.000Z", SLAPlan{})
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	ts := got.Targets[0]
	if ts.Count != 2 || ts.FailCount != 0 {
		t.Fatalf("target = %+v", ts)
	}
	if ts.AvgDownloadBps != 50e6 || ts.MinDownloadBps != 0 {
		t.Errorf("download aggregates = %+v, want avg=50e6 min=0 (genuine zero included)", ts)
	}
}

func mbps(v float64) *float64 { return &v }

// TestSummarySLAComplianceNilWithoutPlan checks that sla_compliance stays
// nil, both per-target and overall, when neither the general plan nor any
// target's thresholds set an SLA speed.
func TestSummarySLAComplianceNilWithoutPlan(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	a, _ := s.CreateTarget(ctx, &Target{Name: "home", Engine: "fake", Enabled: true, Lane: "wan"})
	insertResultAt(t, s, a, "fake", "ok", "2026-09-13T10:00:00.000Z")

	got, err := s.Summary(ctx, "2026-09-13T00:00:00.000Z", "2026-09-14T00:00:00.000Z", SLAPlan{})
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if got.SLACompliance != nil {
		t.Errorf("overall SLACompliance = %v, want nil", *got.SLACompliance)
	}
	if got.Targets[0].SLACompliance != nil {
		t.Errorf("target SLACompliance = %v, want nil", *got.Targets[0].SLACompliance)
	}
}

// TestSummarySLAComplianceFromGeneralPlan checks the general plan applies
// to a target with no override, counting only successful results, and
// requiring both download and upload to meet the plan.
func TestSummarySLAComplianceFromGeneralPlan(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	a, _ := s.CreateTarget(ctx, &Target{Name: "home", Engine: "fake", Enabled: true, Lane: "wan"})
	// insertResultAt always uses download=100e6 (100Mbps), upload=50e6 (50Mbps).
	insertResultAt(t, s, a, "fake", "ok", "2026-09-13T10:00:00.000Z")     // meets 90/40 plan
	insertResultAt(t, s, a, "fake", "ok", "2026-09-13T10:05:00.000Z")     // meets 90/40 plan
	insertResultAt(t, s, a, "fake", "failed", "2026-09-13T10:10:00.000Z") // not counted (not ok)
	if _, err := s.InsertResult(ctx, &Result{
		TargetID: &a, TargetName: "home", Engine: "fake", Status: "ok",
		StartedAt: "2026-09-13T10:15:00.000Z", DurationMs: 500,
		OptionsSnapshot: json.RawMessage(`{}`),
		DownloadBps:     80e6, UploadBps: 50e6, // download below 90Mbps plan
	}); err != nil {
		t.Fatal(err)
	}

	got, err := s.Summary(ctx, "2026-09-13T00:00:00.000Z", "2026-09-14T00:00:00.000Z",
		SLAPlan{DownloadMbps: mbps(90), UploadMbps: mbps(40)})
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	ts := got.Targets[0]
	if ts.SLACompliance == nil {
		t.Fatal("target SLACompliance = nil, want a resolved plan")
	}
	// 2 of 3 successful results meet the plan.
	if *ts.SLACompliance < 0.66 || *ts.SLACompliance > 0.67 {
		t.Errorf("target SLACompliance = %v, want ~0.667", *ts.SLACompliance)
	}
	if got.SLACompliance == nil || *got.SLACompliance != *ts.SLACompliance {
		t.Errorf("overall SLACompliance = %v, want it to match the single target", got.SLACompliance)
	}
}

// TestSummarySLAComplianceTargetOverrideWinsOverGeneral checks a target's
// own thresholds.sla_download_mbps/sla_upload_mbps take precedence over
// the general plan, per field independently.
func TestSummarySLAComplianceTargetOverrideWinsOverGeneral(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	// Override only the download plan (200Mbps, which insertResultAt's
	// 100Mbps never meets); upload falls back to the general 40Mbps plan
	// (which insertResultAt's 50Mbps meets).
	thresholds, _ := json.Marshal(map[string]any{"sla_download_mbps": 200})
	a, _ := s.CreateTarget(ctx, &Target{Name: "home", Engine: "fake", Enabled: true, Lane: "wan",
		Thresholds: thresholds})
	insertResultAt(t, s, a, "fake", "ok", "2026-09-13T10:00:00.000Z")

	got, err := s.Summary(ctx, "2026-09-13T00:00:00.000Z", "2026-09-14T00:00:00.000Z",
		SLAPlan{DownloadMbps: mbps(90), UploadMbps: mbps(40)})
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	ts := got.Targets[0]
	if ts.SLACompliance == nil {
		t.Fatal("SLACompliance = nil, want a resolved plan")
	}
	// download_bps=100e6 fails the 200Mbps override; the general
	// 40Mbps upload plan is met, but both must hold.
	if *ts.SLACompliance != 0 {
		t.Errorf("SLACompliance = %v, want 0 (download override not met)", *ts.SLACompliance)
	}
}

// TestSummarySLAComplianceWeightedOverall checks the top-level
// sla_compliance is weighted by each target's own successful-result
// count, not a simple average across targets.
func TestSummarySLAComplianceWeightedOverall(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	a, _ := s.CreateTarget(ctx, &Target{Name: "meets", Engine: "fake", Enabled: true, Lane: "wan"})
	b, _ := s.CreateTarget(ctx, &Target{Name: "fails", Engine: "fake", Enabled: true, Lane: "wan"})
	// "meets": 1 result, always compliant (100Mbps/50Mbps vs 90/40 plan).
	insertResultAt(t, s, a, "fake", "ok", "2026-09-13T10:00:00.000Z")
	// "fails": 3 results, all below the download plan.
	for i := 0; i < 3; i++ {
		if _, err := s.InsertResult(ctx, &Result{
			TargetID: &b, TargetName: "fails", Engine: "fake", Status: "ok",
			StartedAt: fmt.Sprintf("2026-09-13T11:0%d:00.000Z", i), DurationMs: 500,
			OptionsSnapshot: json.RawMessage(`{}`),
			DownloadBps:     10e6, UploadBps: 50e6,
		}); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.Summary(ctx, "2026-09-13T00:00:00.000Z", "2026-09-14T00:00:00.000Z",
		SLAPlan{DownloadMbps: mbps(90), UploadMbps: mbps(40)})
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	// 1 compliant out of 4 total successful results, weighted by count
	// (not (1.0+0.0)/2 = 0.5, which would be an unweighted average).
	if got.SLACompliance == nil || *got.SLACompliance != 0.25 {
		t.Errorf("overall SLACompliance = %v, want 0.25 (weighted by count)", got.SLACompliance)
	}
}

// TestSummarySLAPlanZeroOrNegativeTreatedAsUnset covers the fix-round-1
// finding: a plan value <= 0 (at either the general or per-target-override
// level) must resolve as if unset, since PUT 0 is the documented way to
// clear a plan through the settings API (which can't otherwise distinguish
// an omitted field from an explicit JSON null on a plain *float64).
func TestSummarySLAPlanZeroOrNegativeTreatedAsUnset(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	a, _ := s.CreateTarget(ctx, &Target{Name: "home", Engine: "fake", Enabled: true, Lane: "wan"})
	insertResultAt(t, s, a, "fake", "ok", "2026-09-13T10:00:00.000Z")

	for _, plan := range []SLAPlan{
		{DownloadMbps: mbps(0), UploadMbps: mbps(0)},
		{DownloadMbps: mbps(-5), UploadMbps: mbps(-5)},
	} {
		got, err := s.Summary(ctx, "2026-09-13T00:00:00.000Z", "2026-09-14T00:00:00.000Z", plan)
		if err != nil {
			t.Fatalf("Summary: %v", err)
		}
		if got.SLACompliance != nil {
			t.Errorf("plan=%+v: overall SLACompliance = %v, want nil (<=0 treated as unset)", plan, *got.SLACompliance)
		}
		if got.Targets[0].SLACompliance != nil {
			t.Errorf("plan=%+v: target SLACompliance = %v, want nil", plan, *got.Targets[0].SLACompliance)
		}
	}
}

// TestSummarySLATargetOverrideZeroFallsBackToGeneral checks a target
// override of 0 (or negative) doesn't hard-disable that target's plan —
// it falls back to the general plan, same as an unset override.
func TestSummarySLATargetOverrideZeroFallsBackToGeneral(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	thresholds, _ := json.Marshal(map[string]any{"sla_download_mbps": 0})
	a, _ := s.CreateTarget(ctx, &Target{Name: "home", Engine: "fake", Enabled: true, Lane: "wan",
		Thresholds: thresholds})
	insertResultAt(t, s, a, "fake", "ok", "2026-09-13T10:00:00.000Z") // 100/50 Mbps

	got, err := s.Summary(ctx, "2026-09-13T00:00:00.000Z", "2026-09-14T00:00:00.000Z",
		SLAPlan{DownloadMbps: mbps(90), UploadMbps: mbps(40)})
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if got.Targets[0].SLACompliance == nil || *got.Targets[0].SLACompliance != 1 {
		t.Errorf("target SLACompliance = %v, want 1 (falls back to the general 90/40 plan, which the 100/50 result meets)",
			got.Targets[0].SLACompliance)
	}
}

// TestSummarySLASkipsUnmeasuredDirection covers the fix-round-1 finding: a
// direction whose measured value is exactly 0 (the iperf3 engine's way of
// saying "not measured" for a reverse-only or forward-only run, not "0bps
// measured") must be skipped as a criterion, not counted as a miss; if
// every applicable direction is unmeasured, the result is excluded from
// the denominator entirely.
func TestSummarySLASkipsUnmeasuredDirection(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	a, _ := s.CreateTarget(ctx, &Target{Name: "home", Engine: "fake", Enabled: true, Lane: "wan"})
	plan := SLAPlan{DownloadMbps: mbps(90), UploadMbps: mbps(40)}

	// Forward-only (download-only) result: upload_bps=0 means "not
	// measured", so only the download criterion applies. 100Mbps meets
	// the 90Mbps download plan, so this counts as compliant despite
	// upload being 0 (which would otherwise fail the 40Mbps plan).
	if _, err := s.InsertResult(ctx, &Result{
		TargetID: &a, TargetName: "home", Engine: "fake", Status: "ok",
		StartedAt: "2026-09-13T10:00:00.000Z", DurationMs: 500,
		OptionsSnapshot: json.RawMessage(`{}`),
		DownloadBps:     100e6, UploadBps: 0,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Summary(ctx, "2026-09-13T00:00:00.000Z", "2026-09-14T00:00:00.000Z", plan)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if got.Targets[0].SLACompliance == nil || *got.Targets[0].SLACompliance != 1 {
		t.Fatalf("forward-only result: SLACompliance = %v, want 1 (upload unmeasured, not a miss)",
			got.Targets[0].SLACompliance)
	}

	// A second target whose only result has both directions at 0 (neither
	// measured) must be excluded from the denominator entirely: nil, not
	// 0/0 or a miss.
	b, _ := s.CreateTarget(ctx, &Target{Name: "empty", Engine: "fake", Enabled: true, Lane: "wan"})
	if _, err := s.InsertResult(ctx, &Result{
		TargetID: &b, TargetName: "empty", Engine: "fake", Status: "ok",
		StartedAt: "2026-09-13T10:00:00.000Z", DurationMs: 500,
		OptionsSnapshot: json.RawMessage(`{}`),
		DownloadBps:     0, UploadBps: 0,
	}); err != nil {
		t.Fatal(err)
	}
	got, err = s.Summary(ctx, "2026-09-13T00:00:00.000Z", "2026-09-14T00:00:00.000Z", plan)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	var empty TargetSummary
	for _, ts := range got.Targets {
		if ts.TargetName == "empty" {
			empty = ts
		}
	}
	if empty.SLACompliance != nil {
		t.Errorf("both-directions-unmeasured result: SLACompliance = %v, want nil", *empty.SLACompliance)
	}
}

// TestSummarySLAHandlesNullDownloadUploadBps covers the fix-round-1
// finding that results.download_bps/upload_bps are nullable columns
// (InsertResult never writes NULL through the Go API, but the schema
// allows it, e.g. for rows written by other tooling): Summary must not
// error scanning a NULL, and must treat it the same as an unmeasured (0)
// direction rather than crash or miscount.
func TestSummarySLAHandlesNullDownloadUploadBps(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	a, _ := s.CreateTarget(ctx, &Target{Name: "home", Engine: "fake", Enabled: true, Lane: "wan"})
	if _, err := s.Write.ExecContext(ctx, `
		INSERT INTO results(target_id,target_name,engine,options_snapshot,status,started_at,duration_ms,
			download_bps,upload_bps,ping_ms,jitter_ms,packet_loss_pct,bytes_down,bytes_up)
		VALUES (?,?,?,?,?,?,?,NULL,NULL,0,0,0,0,0)`,
		a, "home", "fake", `{}`, "ok", "2026-09-13T10:00:00.000Z", 500); err != nil {
		t.Fatal(err)
	}

	got, err := s.Summary(ctx, "2026-09-13T00:00:00.000Z", "2026-09-14T00:00:00.000Z",
		SLAPlan{DownloadMbps: mbps(90), UploadMbps: mbps(40)})
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if got.Targets[0].SLACompliance != nil {
		t.Errorf("SLACompliance = %v, want nil (NULL speeds treated as unmeasured)", *got.Targets[0].SLACompliance)
	}
}
