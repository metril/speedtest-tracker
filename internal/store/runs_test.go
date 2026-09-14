package store

import (
	"context"
	"errors"
	"testing"
)

func TestRunLifecycle(t *testing.T) {
	s, ctx := openTemp(t), context.Background()

	id, err := s.CreateRun(ctx, "manual", nil)
	if err != nil || id == 0 {
		t.Fatalf("CreateRun: %d %v", id, err)
	}
	r, err := s.GetRun(ctx, id)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if r.Status != "queued" || r.Trigger != "manual" || r.ScheduleID != nil {
		t.Fatalf("got %+v", r)
	}
	if r.StartedAt != nil || r.FinishedAt != nil {
		t.Errorf("timestamps set too early: %+v", r)
	}

	if err := s.SetRunStatus(ctx, id, "running", ""); err != nil {
		t.Fatalf("SetRunStatus running: %v", err)
	}
	r, _ = s.GetRun(ctx, id)
	if r.Status != "running" || r.StartedAt == nil || r.FinishedAt != nil {
		t.Fatalf("after running: %+v", r)
	}

	if err := s.SetRunStatus(ctx, id, "failed", "boom"); err != nil {
		t.Fatalf("SetRunStatus failed: %v", err)
	}
	r, _ = s.GetRun(ctx, id)
	if r.Status != "failed" || r.FinishedAt == nil || r.Error == nil || *r.Error != "boom" {
		t.Fatalf("after failed: %+v", r)
	}

	if err := s.SetRunStatus(ctx, 999, "done", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing run = %v, want ErrNotFound", err)
	}
	if _, err := s.GetRun(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetRun missing = %v, want ErrNotFound", err)
	}
}

func TestListRunsKeysetPagination(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	var ids []int64
	for i := 0; i < 5; i++ {
		id, err := s.CreateRun(ctx, "manual", nil)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}

	page1, next, err := s.ListRuns(ctx, RunFilter{Limit: 2})
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(page1) != 2 || page1[0].ID != ids[4] || page1[1].ID != ids[3] {
		t.Fatalf("page1 = %+v", page1)
	}
	if next != ids[3] {
		t.Fatalf("next = %d, want %d", next, ids[3])
	}

	page2, next2, err := s.ListRuns(ctx, RunFilter{Limit: 2, Cursor: next})
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if len(page2) != 2 || page2[0].ID != ids[2] || page2[1].ID != ids[1] {
		t.Fatalf("page2 = %+v", page2)
	}

	last, next3, err := s.ListRuns(ctx, RunFilter{Limit: 2, Cursor: next2})
	if err != nil {
		t.Fatalf("page3: %v", err)
	}
	if len(last) != 1 || last[0].ID != ids[0] {
		t.Fatalf("page3 = %+v", last)
	}
	if next3 != 0 {
		t.Errorf("next3 = %d, want 0 (exhausted)", next3)
	}
}

func TestQueuedRunForSchedule(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	if _, err := s.Write.ExecContext(ctx,
		`INSERT INTO schedules(id,name,cron) VALUES(7,'nightly','0 3 * * *')`); err != nil {
		t.Fatal(err)
	}
	sched := int64(7)

	if _, ok, err := s.QueuedRunForSchedule(ctx, sched); err != nil || ok {
		t.Fatalf("empty: ok=%v err=%v", ok, err)
	}
	id, _ := s.CreateRun(ctx, "cron", &sched)
	got, ok, err := s.QueuedRunForSchedule(ctx, sched)
	if err != nil || !ok || got != id {
		t.Fatalf("queued: %d %v %v", got, ok, err)
	}
	if err := s.SetRunStatus(ctx, id, "done", ""); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.QueuedRunForSchedule(ctx, sched); ok {
		t.Error("finished run still reported as queued")
	}
}

// backdateRun overwrites a run's started_at/finished_at directly, so
// retention tests can simulate old runs without waiting.
func backdateRun(t *testing.T, s *Store, runID int64, at string) {
	t.Helper()
	if _, err := s.Write.ExecContext(context.Background(),
		`UPDATE runs SET started_at=?, finished_at=? WHERE id=?`, at, at, runID); err != nil {
		t.Fatalf("backdateRun: %v", err)
	}
}

func TestPruneRunsBeforeKeepsRunsWithResults(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	withResult, _ := s.CreateRun(ctx, "cron", nil)
	s.SetRunStatus(ctx, withResult, "running", "")
	s.SetRunStatus(ctx, withResult, "done", "")
	insertResultForRun(t, s, withResult, "2030-01-01T00:00:00.000Z")
	empty, _ := s.CreateRun(ctx, "cron", nil)
	s.SetRunStatus(ctx, empty, "running", "")
	s.SetRunStatus(ctx, empty, "done", "")
	backdateRun(t, s, empty, "2020-01-01T00:00:00.000Z")
	backdateRun(t, s, withResult, "2020-01-01T00:00:00.000Z")

	n, err := s.PruneRunsBefore(ctx, "2021-01-01T00:00:00.000Z", 100)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("deleted %d runs, want 1 (the one with no results)", n)
	}
	if _, err := s.GetRun(ctx, withResult); err != nil {
		t.Fatalf("run holding results was deleted: %v", err)
	}
	if _, err := s.GetRun(ctx, empty); !errors.Is(err, ErrNotFound) {
		t.Fatalf("empty run not pruned: %v", err)
	}
}

// TestSetRunStatusRejectsTransitionOutOfTerminal covers the terminal-state
// guard: once a run is done/failed/canceled/skipped, no further status
// write may change it.
func TestSetRunStatusRejectsTransitionOutOfTerminal(t *testing.T) {
	s, ctx := openTemp(t), context.Background()

	for _, terminal := range []string{"done", "failed", "canceled", "skipped"} {
		id, err := s.CreateRun(ctx, "manual", nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.SetRunStatus(ctx, id, terminal, ""); err != nil {
			t.Fatalf("set %s: %v", terminal, err)
		}
		if err := s.SetRunStatus(ctx, id, "running", ""); !errors.Is(err, ErrInvalidTransition) {
			t.Errorf("transition out of %s = %v, want ErrInvalidTransition", terminal, err)
		}
		r, err := s.GetRun(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if r.Status != terminal {
			t.Errorf("status after rejected transition = %q, want unchanged %q", r.Status, terminal)
		}
	}
}
