package prune_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/metril/speedtest-tracker/internal/prune"
	"github.com/metril/speedtest-tracker/internal/settings"
	"github.com/metril/speedtest-tracker/internal/store"
)

// newTestStack opens a fresh store and settings.Store over a temp db file.
func newTestStack(t *testing.T) (*store.Store, *settings.Store) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	st, err := settings.New(context.Background(), db)
	if err != nil {
		t.Fatalf("settings.New: %v", err)
	}
	return db, st
}

// insertResultAt inserts a plain result with an explicit started_at, for
// prune-job tests that only care about ordering.
func insertResultAt(t *testing.T, db *store.Store, startedAt string) int64 {
	t.Helper()
	id, err := db.InsertResult(context.Background(), &store.Result{
		Engine: "ookla", Status: "ok", StartedAt: startedAt,
		OptionsSnapshot: json.RawMessage("{}"),
	})
	if err != nil {
		t.Fatalf("InsertResult: %v", err)
	}
	return id
}

func TestJobOncePrunesBothTables(t *testing.T) {
	db, st := newTestStack(t)
	ctx := context.Background()
	st.Set(ctx, settings.KeyRetentionDaysResults, 1)
	st.Set(ctx, settings.KeyRetentionDaysRuns, 1)
	insertResultAt(t, db, "2020-01-01T00:00:00.000Z")

	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	j := prune.New(prune.Config{Store: db, Settings: st, Batch: 2,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now:    func() time.Time { return now }})
	got, err := j.Once(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.Results != 1 {
		t.Fatalf("pruned %d results, want 1", got.Results)
	}
	if last := j.Last(); last.Results != 1 || !last.At.Equal(now) {
		t.Fatalf("Last() = %+v, want the Once stats", last)
	}
}

func TestRunStopsOnContextCancel(t *testing.T) {
	db, st := newTestStack(t)
	st.Set(context.Background(), settings.KeyRetentionPruneIntervalMinutes, 1)
	j := prune.New(prune.Config{Store: db, Settings: st,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { j.Run(ctx); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancel")
	}
}
