package scheduler

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/metril/speedtest-tracker/internal/runner"
	"github.com/metril/speedtest-tracker/internal/store"
)

// stubEnqueuer records requests instead of running tests.
type stubEnqueuer struct {
	mu   sync.Mutex
	reqs []runner.RunRequest
	err  error
	next int64
}

func (s *stubEnqueuer) Enqueue(_ context.Context, req runner.RunRequest) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return 0, s.err
	}
	s.next++
	return s.next, nil
}

func (s *stubEnqueuer) calls() []runner.RunRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]runner.RunRequest(nil), s.reqs...)
}

func newTestScheduler(t *testing.T) (*Scheduler, *store.Store, *stubEnqueuer) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "sched.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	enq := &stubEnqueuer{}
	s := New(Config{
		Store:  db,
		Runner: enq,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.Stop(ctx)
	})
	return s, db, enq
}

// seed creates two targets and a schedule over them in the given order.
func seed(t *testing.T, db *store.Store, name, expr string, enabled bool) (int64, []int64) {
	t.Helper()
	ctx := context.Background()
	ids := []int64{}
	for _, n := range []string{name + "-t1", name + "-t2"} {
		id, err := db.CreateTarget(ctx, &store.Target{Name: n, Engine: "fake", Enabled: true, QueueID: 1})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	ordered := []int64{ids[1], ids[0]}
	sid, err := db.CreateSchedule(ctx, &store.Schedule{
		Name: name, Cron: expr, Enabled: enabled, Timezone: "UTC", TargetIDs: ordered})
	if err != nil {
		t.Fatal(err)
	}
	return sid, ordered
}

func TestReloadRegistersOnlyEnabledSchedules(t *testing.T) {
	s, db, _ := newTestScheduler(t)
	on, _ := seed(t, db, "on", "@hourly", true)
	off, _ := seed(t, db, "off", "@hourly", false)
	if err := s.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.Entries() != 1 {
		t.Fatalf("entries = %d, want 1", s.Entries())
	}
	if _, ok := s.Next(on); !ok {
		t.Fatal("Next(enabled) not found")
	}
	if _, ok := s.Next(off); ok {
		t.Fatal("Next(disabled) found, want missing")
	}
}

func TestReloadIsIdempotentAndDropsRemovedSchedules(t *testing.T) {
	s, db, _ := newTestScheduler(t)
	sid, _ := seed(t, db, "one", "@hourly", true)
	ctx := context.Background()
	if err := s.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	if s.Entries() != 1 {
		t.Fatalf("entries after double reload = %d, want 1", s.Entries())
	}
	if err := db.DeleteSchedule(ctx, sid); err != nil {
		t.Fatal(err)
	}
	if err := s.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	if s.Entries() != 0 {
		t.Fatalf("entries after delete+reload = %d, want 0", s.Entries())
	}
}

func TestReloadSkipsInvalidCronWithoutFailing(t *testing.T) {
	s, db, _ := newTestScheduler(t)
	seed(t, db, "broken", "@hourly", true)
	if _, err := db.Write.ExecContext(context.Background(),
		`UPDATE schedules SET cron='nonsense' WHERE name='broken'`); err != nil {
		t.Fatal(err)
	}
	if err := s.Reload(context.Background()); err != nil {
		t.Fatalf("Reload = %v, want nil (bad expression is logged, not fatal)", err)
	}
	if s.Entries() != 0 {
		t.Fatalf("entries = %d, want 0", s.Entries())
	}
}

func TestFireEnqueuesCronRunWithOrderedTargets(t *testing.T) {
	s, db, enq := newTestScheduler(t)
	sid, ordered := seed(t, db, "nightly", "@hourly", true)
	sc, err := db.GetSchedule(context.Background(), sid)
	if err != nil {
		t.Fatal(err)
	}
	s.fire(*sc)

	calls := enq.calls()
	if len(calls) != 1 {
		t.Fatalf("enqueue calls = %d, want 1", len(calls))
	}
	got := calls[0]
	if got.Trigger != "cron" || got.ScheduleID == nil || *got.ScheduleID != sid {
		t.Fatalf("request = %+v", got)
	}
	if len(got.TargetIDs) != 2 || got.TargetIDs[0] != ordered[0] || got.TargetIDs[1] != ordered[1] {
		t.Fatalf("target ids = %v, want %v", got.TargetIDs, ordered)
	}
}

func TestFireWritesSkippedRunWhenPreviousRunStillInFlight(t *testing.T) {
	s, db, enq := newTestScheduler(t)
	ctx := context.Background()
	sid, _ := seed(t, db, "busy", "@hourly", true)
	if _, err := db.CreateRun(ctx, "cron", &sid); err != nil { // still queued
		t.Fatal(err)
	}
	sc, err := db.GetSchedule(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	s.fire(*sc)

	if len(enq.calls()) != 0 {
		t.Fatalf("enqueued %d runs, want 0", len(enq.calls()))
	}
	runs, _, err := db.ListRuns(ctx, store.RunFilter{ScheduleID: &sid, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	var skipped int
	for _, r := range runs {
		if r.Status == "skipped" {
			skipped++
		}
	}
	if skipped != 1 {
		t.Fatalf("skipped runs = %d, want 1 (runs: %+v)", skipped, runs)
	}
}

func TestFireWritesSkippedRunOnQueueFull(t *testing.T) {
	s, db, enq := newTestScheduler(t)
	enq.err = runner.ErrQueueFull
	ctx := context.Background()
	sid, _ := seed(t, db, "full", "@hourly", true)
	sc, err := db.GetSchedule(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	s.fire(*sc)

	runs, _, err := db.ListRuns(ctx, store.RunFilter{ScheduleID: &sid, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Status != "skipped" ||
		runs[0].Error == nil || !strings.Contains(*runs[0].Error, "queue full") {
		t.Fatalf("runs = %+v", runs)
	}
}

func TestFireDoesNotWriteSkippedRunOnUnknownError(t *testing.T) {
	s, db, enq := newTestScheduler(t)
	enq.err = errors.New("boom")
	ctx := context.Background()
	sid, _ := seed(t, db, "err", "@hourly", true)
	sc, err := db.GetSchedule(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	s.fire(*sc)
	runs, _, err := db.ListRuns(ctx, store.RunFilter{ScheduleID: &sid, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Fatalf("runs = %+v, want none", runs)
	}
}

func TestCronEntryFiresEndToEnd(t *testing.T) {
	s, db, enq := newTestScheduler(t)
	seed(t, db, "fast", "@every 1s", true)
	if err := s.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if len(enq.calls()) > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("cron entry never fired within 4s")
}

func TestStopIsSafeBeforeAnyReload(t *testing.T) {
	s, _, _ := newTestScheduler(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Stop(ctx); err != nil {
		t.Fatalf("Stop = %v", err)
	}
}

// TestConcurrentReloadsLeaveExactlyOneLiveCron guards against R2 stopping
// R1's not-yet-started cron (a leak) or two Reloads racing to swap s.cron.
// reloadMu serializes them, so after all finish exactly one cron survives
// with the right entry count.
func TestConcurrentReloadsLeaveExactlyOneLiveCron(t *testing.T) {
	s, db, _ := newTestScheduler(t)
	seed(t, db, "one", "@hourly", true)
	seed(t, db, "two", "@hourly", true)

	const n = 10
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			if err := s.Reload(context.Background()); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()

	if got := s.Entries(); got != 2 {
		t.Fatalf("entries = %d, want 2", got)
	}
	s.mu.Lock()
	c := s.cron
	entries := len(s.entries)
	s.mu.Unlock()
	if c == nil {
		t.Fatal("cron is nil after concurrent reloads")
	}
	if got := len(c.Entries()); got != entries {
		t.Fatalf("live cron entries = %d, want %d (map: %d)", got, entries, entries)
	}
}

// TestReloadAfterStopIsNoop ensures a Reload racing with (or arriving
// after) Stop never starts a cron post-shutdown.
func TestReloadAfterStopIsNoop(t *testing.T) {
	s, db, _ := newTestScheduler(t)
	seed(t, db, "one", "@hourly", true)

	ctx := context.Background()
	if err := s.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	stopCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if err := s.Stop(stopCtx); err != nil {
		t.Fatal(err)
	}

	if err := s.Reload(ctx); err != nil {
		t.Fatalf("Reload after Stop = %v, want nil", err)
	}
	s.mu.Lock()
	c, stopped := s.cron, s.stopped
	s.mu.Unlock()
	if c != nil {
		t.Fatal("Reload after Stop started a cron")
	}
	if !stopped {
		t.Fatal("stopped flag cleared by Reload")
	}
	if got := s.Entries(); got != 0 {
		t.Fatalf("entries after reload post-stop = %d, want 0", got)
	}
}
