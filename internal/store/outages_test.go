package store

import (
	"context"
	"testing"
)

func TestOutagesGroupsConsecutiveFailures(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	tid, _ := s.CreateTarget(ctx, &Target{Name: "home", Engine: "fake", Enabled: true, Lane: "wan"})
	// Three failures 15 minutes apart, then a gap, then one more.
	insertResultAt(t, s, tid, "fake", "failed", "2026-09-13T10:00:00.000Z")
	insertResultAt(t, s, tid, "fake", "failed", "2026-09-13T10:15:00.000Z")
	insertResultAt(t, s, tid, "fake", "degraded", "2026-09-13T10:30:00.000Z")
	insertResultAt(t, s, tid, "fake", "ok", "2026-09-13T10:45:00.000Z")
	insertResultAt(t, s, tid, "fake", "failed", "2026-09-13T14:00:00.000Z")

	inc, err := s.Outages(ctx, "2026-09-13T00:00:00.000Z", "2026-09-14T00:00:00.000Z", 1800)
	if err != nil {
		t.Fatalf("Outages: %v", err)
	}
	if len(inc) != 2 {
		t.Fatalf("incidents = %d: %+v", len(inc), inc)
	}
	if inc[0].StartedAt != "2026-09-13T10:00:00.000Z" || inc[0].EndedAt != "2026-09-13T10:30:00.000Z" || inc[0].Count != 3 {
		t.Errorf("incident0 = %+v", inc[0])
	}
	if inc[0].Kind != "result" || inc[0].TargetName != "t" {
		t.Errorf("incident0 identity = %+v", inc[0])
	}
	if inc[1].Count != 1 {
		t.Errorf("incident1 = %+v", inc[1])
	}
}

func TestOutagesSeparatesTargetsAndIncludesSkippedRuns(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	a, _ := s.CreateTarget(ctx, &Target{Name: "a", Engine: "fake", Enabled: true, Lane: "wan"})
	b, _ := s.CreateTarget(ctx, &Target{Name: "b", Engine: "fake", Enabled: true, Lane: "wan"})
	insertResultAt(t, s, a, "fake", "failed", "2026-09-13T10:00:00.000Z")
	insertResultAt(t, s, b, "fake", "failed", "2026-09-13T10:01:00.000Z")

	sid, err := s.CreateSchedule(ctx, &Schedule{Name: "nightly", Cron: "@hourly", Enabled: true, TargetIDs: []int64{a}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.InsertSkippedRun(ctx, sid, "previous run still in flight"); err != nil {
		t.Fatal(err)
	}

	inc, err := s.Outages(ctx, "2000-01-01T00:00:00.000Z", "2100-01-01T00:00:00.000Z", 1800)
	if err != nil {
		t.Fatal(err)
	}
	var results, skipped int
	for _, i := range inc {
		switch i.Kind {
		case "result":
			results++
		case "skipped":
			skipped++
			if i.TargetName != "nightly" || i.Error == "" {
				t.Errorf("skipped incident = %+v", i)
			}
		}
	}
	if results != 2 || skipped != 1 {
		t.Fatalf("results=%d skipped=%d (%+v)", results, skipped, inc)
	}
}
