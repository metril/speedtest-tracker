package runner

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/metril/speedtest-tracker/internal/engine"
	"github.com/metril/speedtest-tracker/internal/engine/fake"
	"github.com/metril/speedtest-tracker/internal/sse"
	"github.com/metril/speedtest-tracker/internal/store"
)

func newTestRunner(t *testing.T) (*Runner, *store.Store, *sse.Hub) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "runner.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	reg := engine.NewRegistry()
	reg.Register(fake.New())
	hub := sse.NewHub()

	r := New(Config{
		Store:    db,
		Registry: reg,
		Hub:      hub,
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Grace:    2 * time.Second,
	})
	r.Start()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = r.Shutdown(ctx)
	})
	return r, db, hub
}

// waitForRun polls until the run reaches a terminal status.
func waitForRun(t *testing.T, db *store.Store, id int64) *store.Run {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		run, err := db.GetRun(context.Background(), id)
		if err != nil {
			t.Fatalf("GetRun: %v", err)
		}
		switch run.Status {
		case "done", "failed", "canceled", "skipped":
			return run
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("run %d never finished", id)
	return nil
}

func TestEnqueueRunsTargetAndWritesResult(t *testing.T) {
	r, db, hub := newTestRunner(t)
	ctx := context.Background()
	events, cancelSub := hub.Subscribe()
	defer cancelSub()

	tid, err := db.CreateTarget(ctx, &store.Target{
		Name: "home", Engine: "fake", Enabled: true, Lane: "wan",
		Options: json.RawMessage(`{"download_bps":42000000}`),
	})
	if err != nil {
		t.Fatal(err)
	}

	runID, err := r.Enqueue(ctx, RunRequest{Trigger: "manual", TargetIDs: []int64{tid}})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if runID == 0 {
		t.Fatal("Enqueue returned run id 0")
	}

	run := waitForRun(t, db, runID)
	if run.Status != "done" {
		t.Fatalf("run status = %q, want done (err=%v)", run.Status, run.Error)
	}

	results, _, err := db.ListResults(ctx, store.ResultFilter{})
	if err != nil || len(results) != 1 {
		t.Fatalf("results = %d %v", len(results), err)
	}
	got := results[0]
	if got.Status != "ok" || got.Engine != "fake" || got.TargetName != "home" {
		t.Errorf("result = %+v", got)
	}
	if got.DownloadBps != 42_000_000 {
		t.Errorf("download_bps = %v, want 42000000", got.DownloadBps)
	}
	if string(got.OptionsSnapshot) != `{"download_bps":42000000}` {
		t.Errorf("snapshot = %s", got.OptionsSnapshot)
	}
	if got.RunID == nil || *got.RunID != runID {
		t.Errorf("run id = %v, want %d", got.RunID, runID)
	}

	kinds := map[string]int{}
	drain := time.After(time.Second)
	for done := false; !done; {
		select {
		case ev := <-events:
			kinds[ev.Type]++
			if ev.Type == sse.EventResult {
				done = true
			}
		case <-drain:
			done = true
		}
	}
	if kinds[sse.EventProgress] == 0 {
		t.Error("no progress events published")
	}
	if kinds[sse.EventResult] == 0 {
		t.Error("no result event published")
	}
	if kinds[sse.EventRun] == 0 {
		t.Error("no run event published")
	}
}

func TestEnqueueFailedTestMarksRunFailed(t *testing.T) {
	r, db, _ := newTestRunner(t)
	ctx := context.Background()
	tid, _ := db.CreateTarget(ctx, &store.Target{
		Name: "broken", Engine: "fake", Enabled: true, Lane: "wan",
		Options: json.RawMessage(`{"fail":true}`),
	})

	runID, err := r.Enqueue(ctx, RunRequest{Trigger: "manual", TargetIDs: []int64{tid}})
	if err != nil {
		t.Fatal(err)
	}
	run := waitForRun(t, db, runID)
	if run.Status != "failed" {
		t.Errorf("run status = %q, want failed", run.Status)
	}
	results, _, _ := db.ListResults(ctx, store.ResultFilter{})
	if len(results) != 1 || results[0].Status != "failed" || results[0].Error == "" {
		t.Errorf("result = %+v", results)
	}
}

func TestEnqueueUnknownEngineFailsResultNotRun(t *testing.T) {
	r, db, _ := newTestRunner(t)
	ctx := context.Background()
	tid, _ := db.CreateTarget(ctx, &store.Target{
		Name: "nope", Engine: "does-not-exist", Enabled: true, Lane: "wan",
	})
	runID, err := r.Enqueue(ctx, RunRequest{Trigger: "manual", TargetIDs: []int64{tid}})
	if err != nil {
		t.Fatal(err)
	}
	waitForRun(t, db, runID)
	results, _, _ := db.ListResults(ctx, store.ResultFilter{})
	if len(results) != 1 || results[0].Status != "failed" {
		t.Fatalf("want one failed result, got %+v", results)
	}
}

func TestEnqueueRejectsEmptyAndUnknownTargets(t *testing.T) {
	r, _, _ := newTestRunner(t)
	ctx := context.Background()
	if _, err := r.Enqueue(ctx, RunRequest{Trigger: "manual"}); err != ErrNoTargets {
		t.Errorf("empty = %v, want ErrNoTargets", err)
	}
	if _, err := r.Enqueue(ctx, RunRequest{Trigger: "manual", TargetIDs: []int64{404}}); err != ErrNoTargets {
		t.Errorf("unknown = %v, want ErrNoTargets", err)
	}
}

func TestLanesRunInParallel(t *testing.T) {
	r, db, _ := newTestRunner(t)
	ctx := context.Background()
	// Slow the fake engine so serial execution would exceed the deadline.
	slow := &fake.Engine{Steps: 4, Delay: 60 * time.Millisecond}
	r.cfg.Registry.Register(slow)

	wan, _ := db.CreateTarget(ctx, &store.Target{Name: "wan", Engine: "fake", Enabled: true, Lane: "wan"})
	lan, _ := db.CreateTarget(ctx, &store.Target{Name: "lan", Engine: "fake", Enabled: true, Lane: "lan"})

	start := time.Now()
	runID, err := r.Enqueue(ctx, RunRequest{Trigger: "manual", TargetIDs: []int64{wan, lan}})
	if err != nil {
		t.Fatal(err)
	}
	waitForRun(t, db, runID)
	elapsed := time.Since(start)

	// Each target takes ~2*4*60ms = 480ms; two lanes in parallel must beat
	// the ~960ms a serial runner would need.
	if elapsed > 900*time.Millisecond {
		t.Errorf("elapsed = %v, lanes did not run in parallel", elapsed)
	}
	results, _, _ := db.ListResults(ctx, store.ResultFilter{})
	if len(results) != 2 {
		t.Errorf("results = %d, want 2", len(results))
	}
}

func TestCancelStopsRun(t *testing.T) {
	r, db, _ := newTestRunner(t)
	ctx := context.Background()
	r.cfg.Registry.Register(&fake.Engine{Steps: 20, Delay: 30 * time.Millisecond})
	tid, _ := db.CreateTarget(ctx, &store.Target{Name: "slow", Engine: "fake", Enabled: true, Lane: "wan"})

	runID, err := r.Enqueue(ctx, RunRequest{Trigger: "manual", TargetIDs: []int64{tid}})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if !r.Cancel(runID) {
		t.Fatal("Cancel returned false for an in-flight run")
	}
	run := waitForRun(t, db, runID)
	if run.Status != "canceled" {
		t.Errorf("status = %q, want canceled", run.Status)
	}
	if r.Cancel(99999) {
		t.Error("Cancel of an unknown run returned true")
	}
}

func TestEnqueueDedupesQueuedSchedule(t *testing.T) {
	r, db, _ := newTestRunner(t)
	ctx := context.Background()
	if _, err := db.Write.ExecContext(ctx,
		`INSERT INTO schedules(id,name,cron) VALUES(3,'n','* * * * *')`); err != nil {
		t.Fatal(err)
	}
	r.cfg.Registry.Register(&fake.Engine{Steps: 20, Delay: 30 * time.Millisecond})
	tid, _ := db.CreateTarget(ctx, &store.Target{Name: "s", Engine: "fake", Enabled: true, Lane: "wan"})
	sched := int64(3)

	first, err := r.Enqueue(ctx, RunRequest{Trigger: "cron", ScheduleID: &sched, TargetIDs: []int64{tid}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := r.Enqueue(ctx, RunRequest{Trigger: "cron", ScheduleID: &sched, TargetIDs: []int64{tid}})
	if err != nil {
		t.Fatalf("dedupe should not error: %v", err)
	}
	if second != first {
		t.Errorf("second = %d, want the already-queued run %d", second, first)
	}
	r.Cancel(first)
	waitForRun(t, db, first)
}

func TestShutdownMarksInflightCanceled(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "shutdown.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	reg := engine.NewRegistry()
	reg.Register(&fake.Engine{Steps: 50, Delay: 40 * time.Millisecond})
	r := New(Config{
		Store: db, Registry: reg, Hub: sse.NewHub(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Grace:  150 * time.Millisecond,
	})
	r.Start()

	ctx := context.Background()
	tid, _ := db.CreateTarget(ctx, &store.Target{Name: "slow", Engine: "fake", Enabled: true, Lane: "wan"})
	runID, err := r.Enqueue(ctx, RunRequest{Trigger: "manual", TargetIDs: []int64{tid}})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(60 * time.Millisecond)

	shutdownCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := r.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	run, err := db.GetRun(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != "canceled" && run.Status != "done" && run.Status != "failed" {
		t.Errorf("status after shutdown = %q, want terminal", run.Status)
	}
	if _, err := r.Enqueue(ctx, RunRequest{Trigger: "manual", TargetIDs: []int64{tid}}); err != ErrShuttingDown {
		t.Errorf("Enqueue after shutdown = %v, want ErrShuttingDown", err)
	}
}
