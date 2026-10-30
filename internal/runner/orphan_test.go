package runner

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func TestCancelOrphanedRun(t *testing.T) {
	t.Parallel()
	r, db, _ := newTestRunner(t)
	ctx := context.Background()

	id, err := db.CreateRun(ctx, "manual", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.SetRunStatus(ctx, id, "running", "")
	if !r.Cancel(id) {
		t.Fatal("Cancel of orphaned running run = false, want true")
	}
	run, _ := db.GetRun(ctx, id)
	if run.Status != "canceled" || run.FinishedAt == nil {
		t.Fatalf("got %+v", run)
	}
	if r.Cancel(id) {
		t.Error("Cancel of already-canceled run = true, want false")
	}
	if r.Cancel(99999) {
		t.Error("Cancel of unknown run = true, want false")
	}
}

func TestCancelDuringFinalWriteKeepsDone(t *testing.T) {
	t.Parallel()
	r, db, _ := newTestRunner(t)
	entered, release := make(chan struct{}), make(chan struct{})
	r.statusHook = func(_ int64, status string, _ int) error {
		if status == "done" {
			close(entered)
			<-release
		}
		return nil
	}
	id := enqueueFakeTarget(t, r, db)
	<-entered
	if r.Cancel(id) {
		t.Error("Cancel during final status write = true, want false")
	}
	close(release)
	if run := waitForRun(t, db, id); run.Status != "done" {
		t.Fatalf("status = %q, want done", run.Status)
	}
}

func TestSetRunStatusRetrySucceedsAfterOneFailure(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	r, db := newTestRunnerWith(t, func(c *Config) {
		c.Logger = slog.New(slog.NewTextHandler(&buf, nil))
	})
	r.statusHook = func(_ int64, status string, attempt int) error {
		if status == "done" && attempt == 0 {
			return errors.New("transient")
		}
		return nil
	}
	id := enqueueFakeTarget(t, r, db)
	if run := waitForRun(t, db, id); run.Status != "done" {
		t.Fatalf("status = %q, want done", run.Status)
	}
	if n := strings.Count(buf.String(), "level=ERROR"); n != 1 {
		t.Errorf("error logs = %d, want 1: %q", n, buf.String())
	}
}

func TestSetRunStatusRetryLogsError(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	r, db := newTestRunnerWith(t, func(c *Config) {
		c.Logger = slog.New(slog.NewTextHandler(&buf, nil))
	})
	id, _ := db.CreateRun(context.Background(), "manual", nil)
	db.Close()
	if err := r.setRunStatus(id, "failed", "x"); err == nil {
		t.Fatal("want error on closed store")
	}
	if !strings.Contains(buf.String(), "level=ERROR") {
		t.Errorf("no error logged: %q", buf.String())
	}
}
