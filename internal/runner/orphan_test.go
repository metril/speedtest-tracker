package runner

import (
	"bytes"
	"context"
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
