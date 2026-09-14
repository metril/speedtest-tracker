// Package prune deletes results and runs older than the configured
// retention windows. It runs on its own goroutine, in small batches so a
// large backlog never holds the single write connection for long, and
// re-reads its schedule whenever the retention settings change.
package prune

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/metril/speedtest-tracker/internal/settings"
	"github.com/metril/speedtest-tracker/internal/store"
)

// timeFormat is the timestamp format used throughout the schema.
const timeFormat = "2006-01-02T15:04:05.000Z"

// Stats is the outcome of one pruning pass.
type Stats struct {
	At      time.Time `json:"at"`
	Results int64     `json:"results"`
	Runs    int64     `json:"runs"`
	Err     string    `json:"error,omitempty"`
}

// Config configures a Job.
type Config struct {
	Store    *store.Store
	Settings *settings.Store
	Logger   *slog.Logger
	Batch    int // rows per delete batch, default 1000
	Now      func() time.Time
}

// Job periodically prunes results and runs according to the retention
// settings, and remembers the outcome of its most recent pass.
type Job struct {
	cfg Config

	mu   sync.Mutex
	last Stats
}

// New returns a ready-to-run Job, applying defaults for any zero-valued
// Config field.
func New(cfg Config) *Job {
	if cfg.Batch <= 0 {
		cfg.Batch = 1000
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Job{cfg: cfg}
}

// Last returns the stats of the most recently completed pruning pass (the
// zero value if none has run yet).
func (j *Job) Last() Stats {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.last
}

// Once runs a single pruning pass to completion and records its stats.
func (j *Job) Once(ctx context.Context) (Stats, error) {
	g, err := j.cfg.Settings.General(ctx)
	if err != nil {
		return Stats{}, err
	}
	now := j.cfg.Now()
	st := Stats{At: now}
	var errs []error

	if g.RetentionDaysResults > 0 {
		cutoff := now.UTC().AddDate(0, 0, -g.RetentionDaysResults).Format(timeFormat)
		n, err := j.drain(ctx, func() (int64, error) {
			return j.cfg.Store.PruneResultsBefore(ctx, cutoff, j.cfg.Batch)
		})
		st.Results = n
		if err != nil {
			errs = append(errs, err)
		}
	}
	if g.RetentionDaysRuns > 0 {
		cutoff := now.UTC().AddDate(0, 0, -g.RetentionDaysRuns).Format(timeFormat)
		n, err := j.drain(ctx, func() (int64, error) {
			return j.cfg.Store.PruneRunsBefore(ctx, cutoff, j.cfg.Batch)
		})
		st.Runs = n
		if err != nil {
			errs = append(errs, err)
		}
	}

	var retErr error
	if len(errs) > 0 {
		retErr = errors.Join(errs...)
		st.Err = retErr.Error()
	}

	j.mu.Lock()
	j.last = st
	j.mu.Unlock()

	return st, retErr
}

// drain repeatedly calls del until it reports 0 rows deleted or an error,
// summing rows deleted. It checks ctx between batches so a cancellation
// stops the loop promptly instead of chewing through a large backlog.
func (j *Job) drain(ctx context.Context, del func() (int64, error)) (int64, error) {
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		n, err := del()
		total += n
		if err != nil {
			return total, err
		}
		if n == 0 {
			return total, nil
		}
	}
}

// minInterval is the floor applied to a configured or default prune
// interval, so a misconfigured value of 0 or less never turns into a busy
// loop.
const minInterval = time.Minute

// intervalFor reads the current prune interval from settings, clamped to
// at least minInterval.
func (j *Job) intervalFor(ctx context.Context) time.Duration {
	g, err := j.cfg.Settings.General(ctx)
	if err != nil {
		j.cfg.Logger.Error("prune: read settings for interval", "error", err)
		return minInterval
	}
	d := time.Duration(g.RetentionPruneIntervalMinutes) * time.Minute
	if d < minInterval {
		d = minInterval
	}
	return d
}

// Run runs pruning passes on a ticker until ctx is canceled, re-reading the
// prune interval whenever the corresponding setting changes. It returns
// only after ctx is done.
func (j *Job) Run(ctx context.Context) {
	changes, cancel := j.cfg.Settings.Subscribe()
	defer cancel()

	// Run one pass immediately so a restart does not leave retention
	// unenforced for up to a full interval.
	if ctx.Err() == nil {
		if st, err := j.Once(ctx); err != nil {
			j.cfg.Logger.Error("prune: pass failed", "error", err,
				"results", st.Results, "runs", st.Runs)
		} else {
			j.cfg.Logger.Info("prune: pass complete",
				"results", st.Results, "runs", st.Runs)
		}
	}

	ticker := time.NewTicker(j.intervalFor(ctx))
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			st, err := j.Once(ctx)
			if err != nil {
				j.cfg.Logger.Error("prune: pass failed", "error", err,
					"results", st.Results, "runs", st.Runs)
			} else {
				j.cfg.Logger.Info("prune: pass complete",
					"results", st.Results, "runs", st.Runs)
			}
		case key, ok := <-changes:
			if !ok {
				return
			}
			if key == settings.KeyRetentionPruneIntervalMinutes {
				ticker.Reset(j.intervalFor(ctx))
			}
		}
	}
}
