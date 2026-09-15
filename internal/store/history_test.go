package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestHistoryBucketsGroupsByInterval(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	tid, _ := s.CreateTarget(ctx, &Target{Name: "home", Engine: "fake", Enabled: true, QueueID: 1})
	// Two results in the 10:00 hour, one in the 11:00 hour, one failure.
	insertResultAt(t, s, tid, "fake", "ok", "2026-09-13T10:05:00.000Z")
	insertResultAt(t, s, tid, "fake", "ok", "2026-09-13T10:45:00.000Z")
	insertResultAt(t, s, tid, "fake", "failed", "2026-09-13T11:05:00.000Z")

	pts, err := s.HistoryBuckets(ctx, tid, "2026-09-13T00:00:00.000Z", "2026-09-14T00:00:00.000Z", 3600)
	if err != nil {
		t.Fatalf("HistoryBuckets: %v", err)
	}
	if len(pts) != 2 {
		t.Fatalf("buckets = %d, want 2 (%+v)", len(pts), pts)
	}
	if pts[0].BucketStart != "2026-09-13T10:00:00.000Z" || pts[0].Count != 2 || pts[0].FailCount != 0 {
		t.Errorf("bucket0 = %+v", pts[0])
	}
	if pts[0].AvgDownloadBps != 100e6 || pts[0].MaxPingMs != 12.5 {
		t.Errorf("bucket0 aggregates = %+v", pts[0])
	}
	if pts[1].BucketStart != "2026-09-13T11:00:00.000Z" || pts[1].FailCount != 1 {
		t.Errorf("bucket1 = %+v", pts[1])
	}
}

func TestHistoryBucketsExcludesOtherTargetsAndRange(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	a, _ := s.CreateTarget(ctx, &Target{Name: "a", Engine: "fake", Enabled: true, QueueID: 1})
	b, _ := s.CreateTarget(ctx, &Target{Name: "b", Engine: "fake", Enabled: true, QueueID: 1})
	insertResultAt(t, s, a, "fake", "ok", "2026-09-13T10:05:00.000Z")
	insertResultAt(t, s, b, "fake", "ok", "2026-09-13T10:06:00.000Z")
	insertResultAt(t, s, a, "fake", "ok", "2026-09-01T10:05:00.000Z")

	pts, err := s.HistoryBuckets(ctx, a, "2026-09-13T00:00:00.000Z", "2026-09-14T00:00:00.000Z", 3600)
	if err != nil {
		t.Fatal(err)
	}
	if len(pts) != 1 || pts[0].Count != 1 {
		t.Fatalf("pts = %+v", pts)
	}
}

func TestHistoryBucketsExcludesFailedRowFromAvgButCountsIt(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	tid, _ := s.CreateTarget(ctx, &Target{Name: "home", Engine: "fake", Enabled: true, QueueID: 1})
	insertResultAt(t, s, tid, "fake", "ok", "2026-09-13T10:05:00.000Z")
	// A failed row with zero metrics: should count toward Count/FailCount
	// but not drag the averages to zero.
	if _, err := s.InsertResult(ctx, &Result{
		TargetID: &tid, TargetName: "home", Engine: "fake", Status: "failed",
		StartedAt: "2026-09-13T10:10:00.000Z", DurationMs: 0,
		OptionsSnapshot: json.RawMessage(`{}`),
	}); err != nil {
		t.Fatalf("InsertResult: %v", err)
	}

	pts, err := s.HistoryBuckets(ctx, tid, "2026-09-13T00:00:00.000Z", "2026-09-14T00:00:00.000Z", 3600)
	if err != nil {
		t.Fatalf("HistoryBuckets: %v", err)
	}
	if len(pts) != 1 {
		t.Fatalf("buckets = %d, want 1 (%+v)", len(pts), pts)
	}
	if pts[0].Count != 2 || pts[0].FailCount != 1 {
		t.Fatalf("bucket = %+v, want Count=2 FailCount=1", pts[0])
	}
	if pts[0].AvgDownloadBps != 100e6 {
		t.Errorf("AvgDownloadBps = %v, want 100e6 (failed row's zero should be excluded)", pts[0].AvgDownloadBps)
	}
}

// TestHistoryBucketsExcludesFailedNonZeroReadingFromAggregates checks that
// status, not the value, decides exclusion: a failed row can still carry a
// nonzero partial reading (e.g. ping succeeded but download failed), and
// that reading must not pollute avg/min/max even though NULLIF(x,0) would
// have let it through.
func TestHistoryBucketsExcludesFailedNonZeroReadingFromAggregates(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	tid, _ := s.CreateTarget(ctx, &Target{Name: "home", Engine: "fake", Enabled: true, QueueID: 1})
	insertResultAt(t, s, tid, "fake", "ok", "2026-09-13T10:05:00.000Z") // download_bps=100e6
	if _, err := s.InsertResult(ctx, &Result{
		TargetID: &tid, TargetName: "home", Engine: "fake", Status: "failed",
		StartedAt: "2026-09-13T10:10:00.000Z", DurationMs: 500,
		OptionsSnapshot: json.RawMessage(`{}`),
		DownloadBps:     999e6, UploadBps: 999e6, PingMs: 999,
	}); err != nil {
		t.Fatalf("InsertResult: %v", err)
	}

	pts, err := s.HistoryBuckets(ctx, tid, "2026-09-13T00:00:00.000Z", "2026-09-14T00:00:00.000Z", 3600)
	if err != nil {
		t.Fatalf("HistoryBuckets: %v", err)
	}
	if len(pts) != 1 || pts[0].Count != 2 || pts[0].FailCount != 1 {
		t.Fatalf("bucket = %+v, want Count=2 FailCount=1", pts[0])
	}
	p := pts[0]
	if p.AvgDownloadBps != 100e6 || p.MinDownloadBps != 100e6 || p.MaxDownloadBps != 100e6 {
		t.Errorf("download aggregates = %+v, want all 100e6 (999e6 failed reading excluded)", p)
	}
	if p.AvgPingMs != 12.5 || p.MaxPingMs != 12.5 {
		t.Errorf("ping aggregates = %+v, want 12.5 (999 failed reading excluded)", p)
	}
}

// TestHistoryBucketsIncludesGenuineZeroFromOkRow checks that an ok row
// reporting an actual 0 (e.g. 0bps observed) is counted, not treated as
// missing the way NULLIF(x,0) used to.
func TestHistoryBucketsIncludesGenuineZeroFromOkRow(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	tid, _ := s.CreateTarget(ctx, &Target{Name: "home", Engine: "fake", Enabled: true, QueueID: 1})
	insertResultAt(t, s, tid, "fake", "ok", "2026-09-13T10:05:00.000Z") // download_bps=100e6
	if _, err := s.InsertResult(ctx, &Result{
		TargetID: &tid, TargetName: "home", Engine: "fake", Status: "ok",
		StartedAt: "2026-09-13T10:10:00.000Z", DurationMs: 500,
		OptionsSnapshot: json.RawMessage(`{}`),
		DownloadBps:     0, UploadBps: 0, PingMs: 0,
	}); err != nil {
		t.Fatalf("InsertResult: %v", err)
	}

	pts, err := s.HistoryBuckets(ctx, tid, "2026-09-13T00:00:00.000Z", "2026-09-14T00:00:00.000Z", 3600)
	if err != nil {
		t.Fatalf("HistoryBuckets: %v", err)
	}
	if len(pts) != 1 || pts[0].Count != 2 || pts[0].FailCount != 0 {
		t.Fatalf("bucket = %+v, want Count=2 FailCount=0", pts[0])
	}
	p := pts[0]
	if p.AvgDownloadBps != 50e6 || p.MinDownloadBps != 0 {
		t.Errorf("download aggregates = %+v, want avg=50e6 min=0 (genuine zero included)", p)
	}
}

func TestBucketSecondsForKeepsPointsUnder500(t *testing.T) {
	for _, span := range []time.Duration{24 * time.Hour, 7 * 24 * time.Hour, 30 * 24 * time.Hour} {
		got := BucketSecondsFor(span)
		if n := int(span.Seconds()) / got; n > 500 {
			t.Errorf("span %s: bucket %ds yields %d points, want <= 500", span, got, n)
		}
		if got < 60 {
			t.Errorf("span %s: bucket %ds is below the 60s floor", span, got)
		}
	}
}
