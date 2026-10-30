package store

import (
	"context"
	"testing"
)

func TestReapOrphanedRuns(t *testing.T) {
	t.Parallel()
	s, ctx := openTemp(t), context.Background()

	queued, _ := s.CreateRun(ctx, "manual", nil)
	running, _ := s.CreateRun(ctx, "manual", nil)
	done, _ := s.CreateRun(ctx, "manual", nil)
	_ = s.SetRunStatus(ctx, running, "running", "")
	_ = s.SetRunStatus(ctx, done, "done", "")

	n, err := s.ReapOrphanedRuns(ctx)
	if err != nil || n != 2 {
		t.Fatalf("ReapOrphanedRuns = %d, %v; want 2", n, err)
	}
	for _, id := range []int64{queued, running} {
		r, _ := s.GetRun(ctx, id)
		if r.Status != "failed" || r.FinishedAt == nil || r.Error == nil || *r.Error != "interrupted by restart" {
			t.Errorf("run %d not reaped: %+v", id, r)
		}
	}
	if r, _ := s.GetRun(ctx, done); r.Status != "done" {
		t.Errorf("terminal run touched: %+v", r)
	}
	if n, _ := s.ReapOrphanedRuns(ctx); n != 0 {
		t.Errorf("second reap = %d, want 0", n)
	}
}
