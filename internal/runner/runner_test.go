package runner

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"strconv"
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
//
// Cancel is called immediately after Enqueue, with no sleep: Cancel sets
// runState.canceled and calls the run's CancelFunc atomically under r.mu,
// and execute() checks st.canceled under that same lock before ever
// writing "running", so the outcome is correct regardless of exactly when
// the lane worker gets around to dequeuing runB relative to when Cancel
// runs (runA's slow engine just guarantees a wide margin). Run with
// -count=N to hammer the race from many different schedules.
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

	// No sleep: cancel runB as soon as it's queued. runA's slow engine
	// (20*2*30ms) keeps the wan lane's single worker busy well past this
	// point, so runB is still sitting in the lane channel either way.
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

// TestShutdownClaimNeverOverwritesFinishedRun proves the atomic
// "delete-to-claim" invariant Shutdown's final loop relies on
// (claimForForceCancel) directly and deterministically: it drives
// finishLane (the exact code a lane worker calls on completion) to win the
// race first — deleting the run from r.runs and persisting "done" — then
// calls the real claimForForceCancel, the same method Shutdown's loop
// calls, against that same run id. Because the entry is already gone, the
// claim must report false and Shutdown's loop would `continue` without
// ever touching the store again, leaving "done" intact.
//
// A real end-to-end reproduction of this race (via a live Shutdown() call
// racing finishLane on the scheduler's own timing) was tried and found too
// narrow a window to trigger reliably either with or without the fix — the
// claim's critical section is only a couple of instructions. Calling the
// production claimForForceCancel method directly (rather than duplicating
// its logic in the test) still exercises the real code Shutdown runs,
// while pinning down the exact interleaving instead of hoping the
// scheduler reproduces it. TestShutdownGaveUpReturnsCtxErr below covers
// the surrounding gaveUp control flow end-to-end.
func TestShutdownClaimNeverOverwritesFinishedRun(t *testing.T) {
	r, db, _ := newTestRunner(t)
	ctx := context.Background()

	runID, err := db.CreateRun(ctx, "manual", nil)
	if err != nil {
		t.Fatal(err)
	}
	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.mu.Lock()
	r.runs[runID] = &runState{ctx: runCtx, cancel: cancel, pending: 1, started: true}
	r.mu.Unlock()

	// finishLane wins the race: exactly what a lane worker calls when its
	// last target finishes successfully. It deletes the run from r.runs
	// and persists "done".
	r.finishLane(runID, false, false)

	run, err := db.GetRun(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != "done" {
		t.Fatalf("setup: run status = %q, want done", run.Status)
	}

	// The real method Shutdown's final loop calls for each stuck id.
	if r.claimForForceCancel(runID) {
		t.Fatal("claimForForceCancel returned true for a run finishLane already claimed")
	}
	// Shutdown's real loop would `continue` here without ever calling
	// SetRunStatus("canceled", ...); confirm the store still says "done".

	run, err = db.GetRun(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != "done" {
		t.Errorf("status after claim attempt = %q, want done (must not be overwritten)", run.Status)
	}
}

// TestClaimForForceCancelIsExclusiveWithFinishLane races the real
// claimForForceCancel (what Shutdown's final loop calls) against the real
// finishLane (what a lane worker calls on completion) for the same run id,
// many times, to prove they can never both believe they own the run.
//
// finishLane always deletes r.runs[id] under r.mu once it decides the
// run's terminal status; claimForForceCancel must do the same atomically,
// so that whichever of the two loses the race sees the entry already gone
// and never acts on it. Before the fix, claimForForceCancel only checked
// presence without deleting, so it could report "still in flight" (true)
// even after finishLane had already deleted the entry and wrote "done" —
// letting Shutdown's caller go on to overwrite it with "canceled". This
// test verifies the exclusivity property directly (rather than hoping a
// live Shutdown() call happens to interleave the same way), by asserting
// that whenever claimForForceCancel reports ownership, finishLane's own
// write must not have landed.
func TestClaimForForceCancelIsExclusiveWithFinishLane(t *testing.T) {
	const trials = 60
	for i := 0; i < trials; i++ {
		r, db, _ := newTestRunner(t)
		ctx := context.Background()

		runID, err := db.CreateRun(ctx, "manual", nil)
		if err != nil {
			t.Fatal(err)
		}
		runCtx, cancel := context.WithCancel(context.Background())
		r.mu.Lock()
		r.runs[runID] = &runState{ctx: runCtx, cancel: cancel, pending: 1, started: true}
		r.mu.Unlock()

		var claimed bool
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			claimed = r.claimForForceCancel(runID)
		}()
		go func() {
			defer wg.Done()
			r.finishLane(runID, false, false)
		}()
		wg.Wait()
		cancel()

		r.mu.Lock()
		_, stillPresent := r.runs[runID]
		r.mu.Unlock()
		if stillPresent {
			t.Fatalf("trial %d: run still present in r.runs after both raced to completion", i)
		}

		run, err := db.GetRun(ctx, runID)
		if err != nil {
			t.Fatal(err)
		}
		if claimed {
			// claimForForceCancel says it deleted the entry itself, which
			// means finishLane's own guarded delete must have found it
			// already gone and returned without writing anything.
			if run.Status == "done" {
				t.Fatalf("trial %d: claimForForceCancel claimed ownership but finishLane still wrote"+
					" done — the two are not mutually exclusive", i)
			}
		} else {
			// Lost the claim race: finishLane must be the one that deleted
			// the entry, so it must have written "done".
			if run.Status != "done" {
				t.Fatalf("trial %d: claim lost the race but status = %q, want done", i, run.Status)
			}
		}
	}
}

// TestShutdownGaveUpReturnsCtxErr checks the gaveUp control flow itself:
// an already-expired ctx must make Shutdown force-cancel in-flight runs
// and return ctx.Err(), without blocking on the (large) Grace period.
func TestShutdownGaveUpReturnsCtxErr(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "gaveup.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	reg := engine.NewRegistry()
	reg.Register(&fake.Engine{Steps: 50, Delay: 40 * time.Millisecond})
	r := New(Config{
		Store: db, Registry: reg, Hub: sse.NewHub(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Grace:  1 * time.Minute, // large: must not be waited out
	})
	r.Start()

	ctx := context.Background()
	tid, _ := db.CreateTarget(ctx, &store.Target{Name: "slow", Engine: "fake", Enabled: true, Lane: "wan"})
	runID, err := r.Enqueue(ctx, RunRequest{Trigger: "manual", TargetIDs: []int64{tid}})
	if err != nil {
		t.Fatal(err)
	}

	// This test is only about the gaveUp/ctx.Err() control flow, not about
	// racing Enqueue against Shutdown (that's covered by
	// TestConcurrentEnqueueAndShutdownNoPanic and
	// TestClaimForForceCancelIsExclusiveWithFinishLane), so wait for the
	// lane worker to actually reach "running" — and be safely blocked deep
	// in the slow engine's loop — before forcing shutdown.
	deadline := time.Now().Add(2 * time.Second)
	for {
		run, err := db.GetRun(ctx, runID)
		if err != nil {
			t.Fatal(err)
		}
		if run.Status == "running" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("run never reached running, status = %q", run.Status)
		}
		time.Sleep(time.Millisecond)
	}

	expiredCtx, cancel := context.WithDeadline(ctx, time.Now().Add(-time.Second))
	defer cancel()

	start := time.Now()
	if err := r.Shutdown(expiredCtx); err != context.DeadlineExceeded {
		t.Fatalf("Shutdown = %v, want context.DeadlineExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("Shutdown took %v, want well under Grace (1m) since ctx was already expired", elapsed)
	}

	run, err := db.GetRun(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != "canceled" && run.Status != "done" && run.Status != "failed" {
		t.Errorf("status after gaveUp shutdown = %q, want terminal", run.Status)
	}
}

// TestExecuteSkipsQueuedRunsAfterShutdownBegins enqueues three runs into one
// lane behind a slow engine, then calls Shutdown right away. The first run
// must already be running (started_at set) by the time Shutdown flips
// r.closing; the other two are still sitting in the lane channel and must
// be dequeued straight into "canceled" (via execute's r.closing check)
// without ever starting an engine or getting started_at set.
func TestExecuteSkipsQueuedRunsAfterShutdownBegins(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "closing-lane.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	reg := engine.NewRegistry()
	reg.Register(&fake.Engine{Steps: 50, Delay: 40 * time.Millisecond})
	r := New(Config{
		Store: db, Registry: reg, Hub: sse.NewHub(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Grace:  200 * time.Millisecond,
	})
	r.Start()
	ctx := context.Background()

	var runIDs []int64
	for i := 0; i < 3; i++ {
		tid, err := db.CreateTarget(ctx, &store.Target{
			Name: "t" + strconv.Itoa(i), Engine: "fake", Enabled: true, Lane: "wan"})
		if err != nil {
			t.Fatal(err)
		}
		id, err := r.Enqueue(ctx, RunRequest{Trigger: "manual", TargetIDs: []int64{tid}})
		if err != nil {
			t.Fatal(err)
		}
		runIDs = append(runIDs, id)
	}

	// Give the lane's single worker time to dequeue and start the first run
	// before Shutdown flips r.closing.
	time.Sleep(60 * time.Millisecond)

	shutdownCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := r.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	first, err := db.GetRun(ctx, runIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	if first.StartedAt == nil {
		t.Error("first run never started")
	}
	if first.Status != "canceled" && first.Status != "done" && first.Status != "failed" {
		t.Errorf("first run status = %q, want terminal", first.Status)
	}

	for i, id := range runIDs[1:] {
		run, err := db.GetRun(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if run.Status != "canceled" {
			t.Errorf("queued run %d status = %q, want canceled", i+1, run.Status)
		}
		if run.StartedAt != nil {
			t.Errorf("queued run %d started_at = %v, want nil (must never have started)", i+1, *run.StartedAt)
		}
	}
}

// TestEnqueuePublishesQueuedBeforeRunning proves clients can never observe
// a "running" run event before its "queued" one: the "queued" publish must
// happen under r.mu, before the job is sent into the lane channel, since a
// worker cannot dequeue (and therefore cannot publish "running") until that
// send has happened.
func TestEnqueuePublishesQueuedBeforeRunning(t *testing.T) {
	r, db, hub := newTestRunner(t)
	ctx := context.Background()
	events, cancelSub := hub.Subscribe()
	defer cancelSub()

	tid, err := db.CreateTarget(ctx, &store.Target{Name: "fast", Engine: "fake", Enabled: true, Lane: "wan"})
	if err != nil {
		t.Fatal(err)
	}

	runID, err := r.Enqueue(ctx, RunRequest{Trigger: "manual", TargetIDs: []int64{tid}})
	if err != nil {
		t.Fatal(err)
	}
	waitForRun(t, db, runID)

	var runStatuses []string
	deadline := time.After(2 * time.Second)
loop:
	for {
		select {
		case ev := <-events:
			if ev.Type != sse.EventRun {
				continue
			}
			var payload struct {
				RunID  int64  `json:"run_id"`
				Status string `json:"status"`
			}
			if err := json.Unmarshal(ev.Data, &payload); err != nil {
				t.Fatal(err)
			}
			if payload.RunID != runID {
				continue
			}
			runStatuses = append(runStatuses, payload.Status)
			switch payload.Status {
			case "done", "failed", "canceled", "skipped":
				break loop
			}
		case <-deadline:
			t.Fatal("timed out waiting for terminal run event")
		}
	}

	if len(runStatuses) == 0 || runStatuses[0] != "queued" {
		t.Fatalf("run event sequence = %v, want first status to be queued", runStatuses)
	}
}

// TestEnqueueSkipsDisabledTargetsExceptManual covers the Target.Enabled
// gate: a scheduled/cron/api trigger must never run a disabled target, but
// a manual run may deliberately target one.
func TestEnqueueSkipsDisabledTargetsExceptManual(t *testing.T) {
	r, db, _ := newTestRunner(t)
	ctx := context.Background()
	tid, err := db.CreateTarget(ctx, &store.Target{Name: "off", Engine: "fake", Enabled: false, Lane: "wan"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := r.Enqueue(ctx, RunRequest{Trigger: "cron", TargetIDs: []int64{tid}}); err != ErrNoTargets {
		t.Errorf("cron on a disabled target = %v, want ErrNoTargets", err)
	}

	runID, err := r.Enqueue(ctx, RunRequest{Trigger: "manual", TargetIDs: []int64{tid}})
	if err != nil {
		t.Fatalf("manual on a disabled target: %v", err)
	}
	run := waitForRun(t, db, runID)
	if run.Status != "done" {
		t.Errorf("manual run on disabled target status = %q, want done", run.Status)
	}
}

// TestEnqueueSkipsDisabledTargetInMixedSet checks a non-manual request that
// names both an enabled and a disabled target: only the enabled one runs.
func TestEnqueueSkipsDisabledTargetInMixedSet(t *testing.T) {
	r, db, _ := newTestRunner(t)
	ctx := context.Background()
	on, err := db.CreateTarget(ctx, &store.Target{Name: "on", Engine: "fake", Enabled: true, Lane: "wan"})
	if err != nil {
		t.Fatal(err)
	}
	off, err := db.CreateTarget(ctx, &store.Target{Name: "off", Engine: "fake", Enabled: false, Lane: "wan"})
	if err != nil {
		t.Fatal(err)
	}

	runID, err := r.Enqueue(ctx, RunRequest{Trigger: "cron", TargetIDs: []int64{on, off}})
	if err != nil {
		t.Fatal(err)
	}
	waitForRun(t, db, runID)
	results, _, err := db.ListResults(ctx, store.ResultFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].TargetName != "on" {
		t.Errorf("results = %+v, want exactly one result for the enabled target", results)
	}
}

// TestCancelNotStartedNeverDoubleWritesTerminalStatus covers the claim
// discipline from Cancel: once Cancel has persisted "canceled" for a run
// that never started, the worker eventually dequeuing that run's lane job
// must find it already gone from r.runs and must not write or publish the
// terminal status again (which would, among other things, bump
// finished_at to a later timestamp).
func TestCancelNotStartedNeverDoubleWritesTerminalStatus(t *testing.T) {
	r, db, _ := newTestRunner(t)
	ctx := context.Background()
	r.cfg.Registry.Register(&fake.Engine{Steps: 20, Delay: 30 * time.Millisecond})
	tidA, err := db.CreateTarget(ctx, &store.Target{Name: "a", Engine: "fake", Enabled: true, Lane: "wan"})
	if err != nil {
		t.Fatal(err)
	}
	tidB, err := db.CreateTarget(ctx, &store.Target{Name: "b", Engine: "fake", Enabled: true, Lane: "wan"})
	if err != nil {
		t.Fatal(err)
	}

	runA, err := r.Enqueue(ctx, RunRequest{Trigger: "manual", TargetIDs: []int64{tidA}})
	if err != nil {
		t.Fatal(err)
	}
	runB, err := r.Enqueue(ctx, RunRequest{Trigger: "manual", TargetIDs: []int64{tidB}})
	if err != nil {
		t.Fatal(err)
	}

	// No sleep: cancel runB while it still sits behind runA's slow engine in
	// the wan lane's channel (same setup as TestCancelQueuedRunNeverStarts).
	if !r.Cancel(runB) {
		t.Fatal("Cancel returned false for a queued run")
	}
	before := waitForRun(t, db, runB)
	if before.Status != "canceled" || before.FinishedAt == nil {
		t.Fatalf("runB after cancel = %+v", before)
	}
	finishedAt := *before.FinishedAt

	// Give the wan lane's worker plenty of time to finish runA and dequeue
	// runB's job; execute() must find runB gone from r.runs and return
	// without touching the store again.
	time.Sleep(300 * time.Millisecond)

	after, err := db.GetRun(ctx, runB)
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != "canceled" {
		t.Errorf("runB status after dequeue = %q, want canceled (unchanged)", after.Status)
	}
	if after.FinishedAt == nil || *after.FinishedAt != finishedAt {
		t.Errorf("runB finished_at changed from %v to %v: status was written a second time", finishedAt, after.FinishedAt)
	}

	r.Cancel(runA)
	waitForRun(t, db, runA)
}
