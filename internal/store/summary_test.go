package store

import (
	"context"
	"encoding/json"
	"testing"
)

func TestSummaryPerTargetAndOverall(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	a, _ := s.CreateTarget(ctx, &Target{Name: "home", Engine: "fake", Enabled: true, Lane: "wan"})
	b, _ := s.CreateTarget(ctx, &Target{Name: "nas", Engine: "fake", Enabled: true, Lane: "lan"})
	insertResultAt(t, s, a, "fake", "ok", "2026-09-13T10:00:00.000Z")
	insertResultAt(t, s, a, "fake", "failed", "2026-09-13T11:00:00.000Z")
	insertResultAt(t, s, b, "fake", "ok", "2026-09-13T12:00:00.000Z")

	got, err := s.Summary(ctx, "2026-09-13T00:00:00.000Z", "2026-09-14T00:00:00.000Z")
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

	got, err := s.Summary(ctx, "2026-09-13T00:00:00.000Z", "2026-09-14T00:00:00.000Z")
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

	got, err := s.Summary(ctx, "2026-09-13T00:00:00.000Z", "2026-09-14T00:00:00.000Z")
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

	got, err := s.Summary(ctx, "2026-09-13T00:00:00.000Z", "2026-09-14T00:00:00.000Z")
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
