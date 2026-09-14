package runner

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
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

// TestEnqueueSnapshotOverridesLiveOptions covers re-execute: when the
// request carries a Snapshots entry for a target, the run uses that
// options document (and stores it as options_snapshot) even though the
// target's live options have since changed.
func TestEnqueueSnapshotOverridesLiveOptions(t *testing.T) {
	r, db, _ := newTestRunner(t)
	ctx := context.Background()

	tid, err := db.CreateTarget(ctx, &store.Target{
		Name: "home", Engine: "fake", Enabled: true, Lane: "wan",
		Options: json.RawMessage(`{"download_bps":1000}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	original := json.RawMessage(`{"download_bps":1000}`)

	// Simulate the target being edited after the original result was
	// captured: the snapshot must still win over the new live options.
	if err := db.UpdateTarget(ctx, &store.Target{
		ID: tid, Name: "home", Engine: "fake", Enabled: true, Lane: "wan",
		Options: json.RawMessage(`{"download_bps":9999}`),
	}); err != nil {
		t.Fatal(err)
	}

	runID, err := r.Enqueue(ctx, RunRequest{
		Trigger:   "reexec",
		TargetIDs: []int64{tid},
		Snapshots: map[int64]json.RawMessage{tid: original},
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	run := waitForRun(t, db, runID)
	if run.Status != "done" {
		t.Fatalf("run status = %q, want done (err=%v)", run.Status, run.Error)
	}

	res, err := db.LatestResultForTarget(ctx, tid)
	if err != nil {
		t.Fatalf("LatestResultForTarget: %v", err)
	}
	if string(res.OptionsSnapshot) != string(original) {
		t.Errorf("options_snapshot = %s, want %s (live options must not win)", res.OptionsSnapshot, original)
	}
	if res.DownloadBps != 1000 {
		t.Errorf("download_bps = %v, want 1000 (engine must have run with the snapshot, not live options)", res.DownloadBps)
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

	// Each target emits 3*Steps+2 = 14 progress events (connecting, Steps
	// ping, Steps download, Steps upload, done), each followed by Delay:
	// 14*60ms = 840ms per target. Two lanes running in parallel must beat
	// the ~1680ms a serial runner would need; 1200ms leaves a comfortable
	// margin over the ~840ms parallel case while staying well under serial.
	if elapsed > 1200*time.Millisecond {
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

// TestConcurrentEnqueueAndShutdownNoPanic hammers Enqueue concurrently with
// Shutdown. Before the fix, Enqueue could send on a lane channel just as
// Shutdown closed it under r.mu, panicking. Every run that does get
// created must still reach a terminal status.
func TestConcurrentEnqueueAndShutdownNoPanic(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "concurrent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	reg := engine.NewRegistry()
	reg.Register(fake.New())
	r := New(Config{
		Store:    db,
		Registry: reg,
		Hub:      sse.NewHub(),
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Grace:    2 * time.Second,
	})
	r.Start()
	ctx := context.Background()

	tid, err := db.CreateTarget(ctx, &store.Target{Name: "t", Engine: "fake", Enabled: true, Lane: "wan"})
	if err != nil {
		t.Fatal(err)
	}

	const n = 20
	var wg sync.WaitGroup
	var mu sync.Mutex
	var runIDs []int64
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := r.Enqueue(ctx, RunRequest{Trigger: "manual", TargetIDs: []int64{tid}})
			if err != nil {
				if err != ErrShuttingDown && err != ErrQueueFull {
					t.Errorf("Enqueue: %v", err)
				}
				return
			}
			mu.Lock()
			runIDs = append(runIDs, id)
			mu.Unlock()
		}()
	}

	shutdownDone := make(chan error, 1)
	go func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		shutdownDone <- r.Shutdown(shutdownCtx)
	}()

	wg.Wait()
	if err := <-shutdownDone; err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	mu.Lock()
	ids := append([]int64(nil), runIDs...)
	mu.Unlock()
	for _, id := range ids {
		run, err := db.GetRun(ctx, id)
		if err != nil {
			t.Fatalf("GetRun(%d): %v", id, err)
		}
		switch run.Status {
		case "done", "failed", "canceled", "skipped":
		default:
			t.Errorf("run %d status = %q, want terminal", id, run.Status)
		}
	}
}

// TestEnqueueDedupeIsRaceFree fires many concurrent Enqueue calls for the
// same schedule and asserts exactly one run is created.
func TestEnqueueDedupeIsRaceFree(t *testing.T) {
	r, db, _ := newTestRunner(t)
	ctx := context.Background()
	if _, err := db.Write.ExecContext(ctx,
		`INSERT INTO schedules(id,name,cron) VALUES(9,'n','* * * * *')`); err != nil {
		t.Fatal(err)
	}
	r.cfg.Registry.Register(&fake.Engine{Steps: 20, Delay: 20 * time.Millisecond})
	tid, _ := db.CreateTarget(ctx, &store.Target{Name: "d", Engine: "fake", Enabled: true, Lane: "wan"})
	sched := int64(9)

	const n = 10
	ids := make([]int64, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id, err := r.Enqueue(ctx, RunRequest{Trigger: "cron", ScheduleID: &sched, TargetIDs: []int64{tid}})
			if err != nil {
				t.Errorf("Enqueue: %v", err)
				return
			}
			ids[i] = id
		}(i)
	}
	wg.Wait()

	first := ids[0]
	for i, id := range ids {
		if id != first {
			t.Errorf("run %d got id %d, want the deduped run %d", i, id, first)
		}
	}
	r.Cancel(first)
	waitForRun(t, db, first)
}

// TestCancelQueuedRunNeverStarts cancels a run while it still sits behind
// another run in the same lane's channel. It must land on "canceled"
// without ever having started_at set, i.e. it never actually ran.
func TestCancelQueuedRunNeverStarts(t *testing.T) {
	r, db, _ := newTestRunner(t)
	ctx := context.Background()
	r.cfg.Registry.Register(&fake.Engine{Steps: 20, Delay: 30 * time.Millisecond})
	tidA, _ := db.CreateTarget(ctx, &store.Target{Name: "a", Engine: "fake", Enabled: true, Lane: "wan"})
	tidB, _ := db.CreateTarget(ctx, &store.Target{Name: "b", Engine: "fake", Enabled: true, Lane: "wan"})

	runA, err := r.Enqueue(ctx, RunRequest{Trigger: "manual", TargetIDs: []int64{tidA}})
	if err != nil {
		t.Fatal(err)
	}
	runB, err := r.Enqueue(ctx, RunRequest{Trigger: "manual", TargetIDs: []int64{tidB}})
	if err != nil {
		t.Fatal(err)
	}

	// runA occupies the wan lane's single worker for ~20*2*30ms; runB is
	// still waiting in the lane channel at this point.
	time.Sleep(20 * time.Millisecond)
	if !r.Cancel(runB) {
		t.Fatal("Cancel returned false for a queued run")
	}

	runBRow := waitForRun(t, db, runB)
	if runBRow.Status != "canceled" {
		t.Errorf("runB status = %q, want canceled", runBRow.Status)
	}
	if runBRow.StartedAt != nil {
		t.Errorf("runB started_at = %v, want nil (never started)", *runBRow.StartedAt)
	}

	r.Cancel(runA)
	waitForRun(t, db, runA)
}
