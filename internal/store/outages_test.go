package store

import (
	"context"
	"testing"
)

func TestOutagesGroupsConsecutiveFailures(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	tid, _ := s.CreateTarget(ctx, &Target{Name: "home", Engine: "fake", Enabled: true, QueueID: 1})
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
	// Most recent first: the lone 14:00 failure comes before the earlier
	// three-result incident.
	if inc[1].StartedAt != "2026-09-13T10:00:00.000Z" || inc[1].EndedAt != "2026-09-13T10:30:00.000Z" || inc[1].Count != 3 {
		t.Errorf("incident1 = %+v", inc[1])
	}
	if inc[1].Kind != "result" || inc[1].TargetName != "t" {
		t.Errorf("incident1 identity = %+v", inc[1])
	}
	if inc[0].Count != 1 || inc[0].StartedAt != "2026-09-13T14:00:00.000Z" {
		t.Errorf("incident0 = %+v", inc[0])
	}
}

// TestOutagesClosesIncidentOnRecovery verifies that an "ok" result ends the
// open incident, so a failure inside gapSeconds after a recovery starts a
// new incident rather than extending the old one.
func TestOutagesClosesIncidentOnRecovery(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	tid, _ := s.CreateTarget(ctx, &Target{Name: "home", Engine: "fake", Enabled: true, QueueID: 1})
	insertResultAt(t, s, tid, "fake", "failed", "2026-09-13T10:00:00.000Z")
	insertResultAt(t, s, tid, "fake", "ok", "2026-09-13T10:05:00.000Z")
	insertResultAt(t, s, tid, "fake", "failed", "2026-09-13T10:10:00.000Z")

	inc, err := s.Outages(ctx, "2026-09-13T00:00:00.000Z", "2026-09-14T00:00:00.000Z", 1800)
	if err != nil {
		t.Fatalf("Outages: %v", err)
	}
	if len(inc) != 2 {
		t.Fatalf("incidents = %d: %+v", len(inc), inc)
	}
	if inc[0].StartedAt != "2026-09-13T10:10:00.000Z" || inc[0].Count != 1 {
		t.Errorf("incident0 = %+v", inc[0])
	}
	if inc[1].StartedAt != "2026-09-13T10:00:00.000Z" || inc[1].Count != 1 {
		t.Errorf("incident1 = %+v", inc[1])
	}
}

// TestOutagesSortedDescendingAndCapped verifies incidents come back newest
// first and are capped at maxIncidents.
func TestOutagesSortedDescendingAndCapped(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	tid, _ := s.CreateTarget(ctx, &Target{Name: "home", Engine: "fake", Enabled: true, QueueID: 1})
	times := []string{
		"2026-09-13T08:00:00.000Z",
		"2026-09-13T09:00:00.000Z",
		"2026-09-13T10:00:00.000Z",
	}
	for _, ts := range times {
		insertResultAt(t, s, tid, "fake", "failed", ts)
		insertResultAt(t, s, tid, "fake", "ok", ts) // immediately close so each is its own incident
	}

	inc, err := s.Outages(ctx, "2026-09-13T00:00:00.000Z", "2026-09-14T00:00:00.000Z", 60)
	if err != nil {
		t.Fatalf("Outages: %v", err)
	}
	if len(inc) != 3 {
		t.Fatalf("incidents = %d: %+v", len(inc), inc)
	}
	for i := 1; i < len(inc); i++ {
		if inc[i-1].StartedAt < inc[i].StartedAt {
			t.Fatalf("incidents not sorted descending: %+v", inc)
		}
	}

	orig := maxIncidents
	maxIncidents = 2
	defer func() { maxIncidents = orig }()
	inc, err = s.Outages(ctx, "2026-09-13T00:00:00.000Z", "2026-09-14T00:00:00.000Z", 60)
	if err != nil {
		t.Fatalf("Outages: %v", err)
	}
	if len(inc) != 2 {
		t.Fatalf("capped incidents = %d: %+v", len(inc), inc)
	}
}

// TestOutagesGapBoundary verifies results exactly gapSeconds apart merge,
// while gapSeconds+1 apart split into separate incidents.
func TestOutagesGapBoundary(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	tid, _ := s.CreateTarget(ctx, &Target{Name: "home", Engine: "fake", Enabled: true, QueueID: 1})
	insertResultAt(t, s, tid, "fake", "failed", "2026-09-13T10:00:00.000Z")
	insertResultAt(t, s, tid, "fake", "failed", "2026-09-13T10:30:00.000Z") // exactly 1800s later: merges

	inc, err := s.Outages(ctx, "2026-09-13T00:00:00.000Z", "2026-09-14T00:00:00.000Z", 1800)
	if err != nil {
		t.Fatalf("Outages: %v", err)
	}
	if len(inc) != 1 || inc[0].Count != 2 {
		t.Fatalf("expected merged incident, got %+v", inc)
	}

	s2, ctx2 := openTemp(t), context.Background()
	tid2, _ := s2.CreateTarget(ctx2, &Target{Name: "home", Engine: "fake", Enabled: true, QueueID: 1})
	insertResultAt(t, s2, tid2, "fake", "failed", "2026-09-13T10:00:00.000Z")
	insertResultAt(t, s2, tid2, "fake", "failed", "2026-09-13T10:30:01.000Z") // 1801s later: splits

	inc2, err := s2.Outages(ctx2, "2026-09-13T00:00:00.000Z", "2026-09-14T00:00:00.000Z", 1800)
	if err != nil {
		t.Fatalf("Outages: %v", err)
	}
	if len(inc2) != 2 || inc2[0].Count != 1 || inc2[1].Count != 1 {
		t.Fatalf("expected split incidents, got %+v", inc2)
	}
}

func TestOutagesSeparatesTargetsAndIncludesSkippedRuns(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	a, _ := s.CreateTarget(ctx, &Target{Name: "a", Engine: "fake", Enabled: true, QueueID: 1})
	b, _ := s.CreateTarget(ctx, &Target{Name: "b", Engine: "fake", Enabled: true, QueueID: 1})
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
