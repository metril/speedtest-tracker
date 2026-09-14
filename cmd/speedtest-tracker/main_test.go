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

	"github.com/metril/speedtest-tracker/internal/engine"
	"github.com/metril/speedtest-tracker/internal/engine/ookla"
	"github.com/metril/speedtest-tracker/internal/settings"
	"github.com/metril/speedtest-tracker/internal/store"
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
	go func() { errCh <- run(ctx, slog.New(slog.NewJSONHandler(io.Discard, nil)), new(slog.LevelVar)) }()

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
	go func() { errCh <- run(ctx, slog.New(slog.NewJSONHandler(io.Discard, nil)), new(slog.LevelVar)) }()

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

func TestParseLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"debug": slog.LevelDebug,
		"INFO":  slog.LevelInfo,
		"warn":  slog.LevelWarn,
		"error": slog.LevelError,
		"":      slog.LevelInfo,
		"weird": slog.LevelInfo,
	}
	for in, want := range cases {
		if got := parseLevel(in); got != want {
			t.Errorf("parseLevel(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestWatchSettingsAppliesLogLevelAndRebuildsEngines(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "watch.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st, err := settings.New(ctx, db)
	if err != nil {
		t.Fatal(err)
	}

	level := new(slog.LevelVar)
	level.Set(slog.LevelInfo)
	reg := engine.NewRegistry()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	servers := ookla.NewServerList("speedtest", time.Hour)

	go watchSettings(ctx, st, level, reg, servers, logger)

	if err := st.Set(ctx, settings.KeyLogLevel, "debug"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for level.Level() != slog.LevelDebug && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if level.Level() != slog.LevelDebug {
		t.Errorf("level = %v, want debug", level.Level())
	}

	if err := st.Set(ctx, settings.KeySpeedtestBin, "/opt/speedtest"); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(2 * time.Second)
	for len(reg.Names()) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	names := reg.Names()
	if len(names) != 4 {
		t.Errorf("engines = %v, want ookla, cloudflare, iperf3, fake", names)
	}
}

func TestBuildEnginesRegistersAll(t *testing.T) {
	got := buildEngines(settings.Engines{SpeedtestBin: "speedtest", Iperf3Bin: "iperf3"})
	for _, name := range []string{"fake", "ookla", "cloudflare", "iperf3"} {
		if _, ok := got[name]; !ok {
			t.Errorf("missing engine %q", name)
		}
	}
}
