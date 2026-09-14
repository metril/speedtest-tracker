package store

import (
	"context"
	"testing"
	"time"
)

func TestHistoryBucketsGroupsByInterval(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	tid, _ := s.CreateTarget(ctx, &Target{Name: "home", Engine: "fake", Enabled: true, Lane: "wan"})
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
	a, _ := s.CreateTarget(ctx, &Target{Name: "a", Engine: "fake", Enabled: true, Lane: "wan"})
	b, _ := s.CreateTarget(ctx, &Target{Name: "b", Engine: "fake", Enabled: true, Lane: "wan"})
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
