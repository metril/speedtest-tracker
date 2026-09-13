package main

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"
)

// freePort asks the OS for an unused port.
func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()
	return l.Addr().String()
}

func TestRunServesHealthzAndShutsDown(t *testing.T) {
	addr := freePort(t)
	t.Setenv("ST_LISTEN", addr)
	t.Setenv("ST_DB_PATH", filepath.Join(t.TempDir(), "main.db"))

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- run(ctx, slog.New(slog.NewJSONHandler(io.Discard, nil))) }()

	var resp *http.Response
	var err error
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err = http.Get("http://" + addr + "/healthz")
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		cancel()
		t.Fatalf("GET /healthz: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("run returned %v, want nil", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("run did not return after context cancel")
	}
}

func TestHealthcheckSucceedsAgainstRunningServer(t *testing.T) {
	addr := freePort(t)
	t.Setenv("ST_LISTEN", addr)
	t.Setenv("ST_DB_PATH", filepath.Join(t.TempDir(), "main.db"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- run(ctx, slog.New(slog.NewJSONHandler(io.Discard, nil))) }()

	deadline := time.Now().Add(5 * time.Second)
	var err error
	for time.Now().Before(deadline) {
		if err = healthcheck(addr); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("healthcheck: %v", err)
	}
	if code := healthcheckMain(addr); code != 0 {
		t.Errorf("healthcheckMain = %d, want 0", code)
	}
}

func TestHealthcheckFailsWhenServerDown(t *testing.T) {
	addr := freePort(t) // nothing listening on this address
	if err := healthcheck(addr); err == nil {
		t.Fatal("healthcheck against a closed port: want error, got nil")
	}
	if code := healthcheckMain(addr); code != 1 {
		t.Errorf("healthcheckMain = %d, want 1", code)
	}
}
