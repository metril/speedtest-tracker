package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/metril/speedtest-tracker/internal/runner"
	"github.com/metril/speedtest-tracker/internal/store"
	"github.com/robfig/cron/v3"
)

// fireTimeout bounds the store/enqueue work one cron callback may do. The
// callback never runs a test — it only enqueues — so this is generous.
const fireTimeout = 10 * time.Second

// Enqueuer is the subset of *runner.Runner the scheduler needs.
type Enqueuer interface {
	Enqueue(ctx context.Context, req runner.RunRequest) (int64, error)
}

// Config configures a Scheduler.
type Config struct {
	Store  *store.Store
	Runner Enqueuer
	Logger *slog.Logger
	Now    func() time.Time
}

// Scheduler owns one cron instance holding one entry per enabled schedule.
// Entries carry their own timezone (CRON_TZ prefix), so a timezone change
// is just another Reload.
type Scheduler struct {
	cfg Config

	// reloadMu serializes Reload (and Reload against Stop) end to end, so a
	// second Reload can't Stop a first Reload's not-yet-started cron
	// (leaking it running forever) and a Reload can't resurrect a cron
	// after Stop has shut the scheduler down.
	reloadMu sync.Mutex

	mu      sync.Mutex
	cron    *cron.Cron
	entries map[int64]cron.EntryID
	stopped bool
}

// New returns a Scheduler. Nothing is scheduled until Reload is called.
func New(cfg Config) *Scheduler {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Scheduler{cfg: cfg, entries: map[int64]cron.EntryID{}}
}

// Reload rebuilds every cron entry from the store. It is called at startup,
// after any schedule mutation and when general.timezone changes. The store
// read happens before the lock is taken, so no lock is ever held across I/O.
func (s *Scheduler) Reload(ctx context.Context) error {
	s.reloadMu.Lock()
	defer s.reloadMu.Unlock()

	schedules, err := s.cfg.Store.ListSchedules(ctx)
	if err != nil {
		return fmt.Errorf("scheduler: load schedules: %w", err)
	}

	// next.Schedule below registers closures that capture sc by value, so
	// every cron callback this Reload creates always fires against the
	// schedule snapshot read by *this* Reload, never a later one's.
	next := cron.New(cron.WithChain(cron.Recover(cron.DefaultLogger)))
	entries := map[int64]cron.EntryID{}
	for _, sc := range schedules {
		if !sc.Enabled {
			continue
		}
		if len(sc.TargetIDs) == 0 {
			s.cfg.Logger.Warn("scheduler: schedule has no targets, not scheduled",
				"schedule", sc.Name, "schedule_id", sc.ID)
			continue
		}
		spec, err := ParseSpec(sc.Cron, sc.Timezone)
		if err != nil {
			// A bad expression must never stop the other schedules from
			// loading; the API validates on save, so this only happens to
			// rows edited outside the app.
			s.cfg.Logger.Error("scheduler: skipping schedule with invalid cron",
				"schedule", sc.Name, "schedule_id", sc.ID, "cron", sc.Cron, "error", err)
			continue
		}
		sc := sc
		entries[sc.ID] = next.Schedule(spec, cron.FuncJob(func() { s.fire(sc) }))
	}

	s.mu.Lock()
	if s.stopped {
		// Stop() ran while we were building next. next was never started,
		// so just drop it instead of starting a cron after shutdown.
		s.mu.Unlock()
		s.cfg.Logger.Warn("scheduler: reload after stop, discarding new cron")
		return nil
	}
	old := s.cron
	s.cron, s.entries = next, entries
	s.mu.Unlock()

	next.Start()
	if old != nil {
		old.Stop()
	}
	s.cfg.Logger.Info("scheduler reloaded", "entries", len(entries))
	return nil
}

// fire is the cron callback: it only enqueues. Anything that prevents the
// run from being queued (a previous run of this schedule still in flight,
// a full lane queue, a shutdown) is recorded as a terminal "skipped" run so
// the outage view can show the gap. Missed fires during downtime are never
// backfilled.
func (s *Scheduler) fire(sc store.Schedule) {
	ctx, cancel := context.WithTimeout(context.Background(), fireTimeout)
	defer cancel()

	// SkipIfStillRunning semantics, but across process restarts and across
	// the whole queue rather than just this cron entry.
	if prev, busy, err := s.cfg.Store.QueuedRunForSchedule(ctx, sc.ID); err != nil {
		s.cfg.Logger.Error("scheduler: dedupe check failed", "schedule", sc.Name, "error", err)
		return
	} else if busy {
		s.skip(ctx, sc, fmt.Sprintf("previous run %d still in flight", prev))
		return
	}

	runID, err := s.cfg.Runner.Enqueue(ctx, runner.RunRequest{
		Trigger:    "cron",
		ScheduleID: &sc.ID,
		TargetIDs:  sc.TargetIDs,
	})
	switch {
	case err == nil:
		s.cfg.Logger.Info("schedule fired", "schedule", sc.Name, "schedule_id", sc.ID, "run_id", runID)
	case errors.Is(err, runner.ErrQueueFull):
		s.skip(ctx, sc, "lane queue full")
	case errors.Is(err, runner.ErrNoTargets):
		s.skip(ctx, sc, "no runnable targets")
	case errors.Is(err, runner.ErrShuttingDown):
		s.skip(ctx, sc, "server shutting down")
	default:
		s.cfg.Logger.Error("scheduler: enqueue failed", "schedule", sc.Name, "error", err)
	}
}

// skip writes the skipped run row and logs it.
func (s *Scheduler) skip(ctx context.Context, sc store.Schedule, reason string) {
	if _, err := s.cfg.Store.InsertSkippedRun(ctx, sc.ID, reason); err != nil {
		s.cfg.Logger.Error("scheduler: write skipped run", "schedule", sc.Name, "error", err)
	}
	s.cfg.Logger.Warn("schedule skipped", "schedule", sc.Name, "schedule_id", sc.ID, "reason", reason)
}

// Next returns the schedule's next fire time, or ok=false when it is not
// currently scheduled (disabled, targetless or invalid).
func (s *Scheduler) Next(scheduleID int64) (time.Time, bool) {
	s.mu.Lock()
	c := s.cron
	id, ok := s.entries[scheduleID]
	s.mu.Unlock()
	if c == nil || !ok {
		return time.Time{}, false
	}
	entry := c.Entry(id)
	if entry.ID == 0 || entry.Next.IsZero() {
		return time.Time{}, false
	}
	return entry.Next, true
}

// Entries reports how many schedules are currently registered.
func (s *Scheduler) Entries() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.entries)
}

// Stop halts the cron instance and waits for any running callback, or
// returns ctx.Err() if that takes too long.
func (s *Scheduler) Stop(ctx context.Context) error {
	s.mu.Lock()
	c := s.cron
	s.cron, s.entries = nil, map[int64]cron.EntryID{}
	s.stopped = true
	s.mu.Unlock()
	if c == nil {
		return nil
	}
	select {
	case <-c.Stop().Done():
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
