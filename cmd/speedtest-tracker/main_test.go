package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/metril/speedtest-tracker/internal/auth"
	"github.com/metril/speedtest-tracker/internal/engine"
	"github.com/metril/speedtest-tracker/internal/engine/fake"
	"github.com/metril/speedtest-tracker/internal/engine/ookla"
	"github.com/metril/speedtest-tracker/internal/iperf3list"
	"github.com/metril/speedtest-tracker/internal/notify"
	"github.com/metril/speedtest-tracker/internal/runner"
	"github.com/metril/speedtest-tracker/internal/scheduler"
	"github.com/metril/speedtest-tracker/internal/settings"
	"github.com/metril/speedtest-tracker/internal/sse"
	"github.com/metril/speedtest-tracker/internal/store"
	"github.com/metril/speedtest-tracker/internal/vlpush"
	"github.com/metril/speedtest-tracker/internal/vmpush"
)

// noTokens is a TokenLookup that never matches, for tests that only care
// about non-token auth modes.
type noTokens struct{}

func (noTokens) LookupToken(context.Context, string) (int64, bool, error) { return 0, false, nil }
func (noTokens) TouchToken(context.Context, int64) error                  { return nil }

// newTestSettings builds a settings.Store backed by a fresh temp-file
// database, closed automatically at test cleanup.
func newTestSettings(t *testing.T) *settings.Store {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	st, err := settings.New(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

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
	sch := scheduler.New(scheduler.Config{Store: db, Runner: runner.New(runner.Config{Store: db, Registry: reg, Hub: sse.NewHub(), Logger: logger}), Logger: logger})
	vm := vmpush.New(vmpush.Config{})
	vm.Start()
	defer vm.Close(context.Background())
	vl := vlpush.New(vlpush.Config{Next: slog.NewJSONHandler(io.Discard, nil)})
	vl.Start()
	defer vl.Close(context.Background())
	nt := notify.New(notify.Config{Store: db, Logger: logger})
	nt.Start()
	defer nt.Close(context.Background())

	am := newAuthAdapter(auth.New(logger, noTokens{}, time.Now))

	ir := iperf3list.New(iperf3list.Config{Store: db, Logger: logger})

	changes, unsubscribe := st.Subscribe()
	defer unsubscribe()
	var metricsEnabled atomic.Bool
	go watchSettings(ctx, st, changes, level, reg, servers, sch, vm, vl, nt, am, &metricsEnabled, ir, logger)

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

func TestSchedulerRunsScheduleEndToEnd(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "e2e.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()

	tid, err := db.CreateTarget(ctx, &store.Target{
		Name: "fake-target", Engine: "fake", Enabled: true, Lane: "wan",
		Options: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateSchedule(ctx, &store.Schedule{
		Name: "every-second", Cron: "@every 1s", Enabled: true, Timezone: "UTC",
		TargetIDs: []int64{tid}}); err != nil {
		t.Fatal(err)
	}

	reg := engine.NewRegistry()
	reg.Register(fake.New())
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rn := runner.New(runner.Config{Store: db, Registry: reg, Hub: sse.NewHub(),
		Logger: logger, Grace: 2 * time.Second})
	rn.Start()
	sch := scheduler.New(scheduler.Config{Store: db, Runner: rn, Logger: logger})
	if err := sch.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = sch.Stop(stopCtx)
		_ = rn.Shutdown(stopCtx)
	}()

	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		results, _, err := db.ListResults(ctx, store.ResultFilter{TargetID: &tid, Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		if len(results) > 0 {
			if results[0].Status != "ok" {
				t.Fatalf("result status = %q, want ok", results[0].Status)
			}
			runs, _, err := db.ListRuns(ctx, store.RunFilter{Limit: 10})
			if err != nil {
				t.Fatal(err)
			}
			if len(runs) == 0 || runs[len(runs)-1].Trigger != "cron" {
				t.Fatalf("runs = %+v, want a cron run", runs)
			}
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("schedule never produced a result within 6s")
}

func TestApplyIntegrationsConfiguresClients(t *testing.T) {
	hits := make(chan string, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits <- r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	db, err := store.Open(filepath.Join(t.TempDir(), "wire.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	st, err := settings.New(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	st.Set(ctx, settings.KeyVMEnabled, true)
	st.Set(ctx, settings.KeyVMURL, srv.URL)
	st.Set(ctx, settings.KeyMetricsEnabled, true)

	vm := vmpush.New(vmpush.Config{})
	vm.Start()
	defer vm.Close(ctx)
	vl := vlpush.New(vlpush.Config{Next: slog.NewJSONHandler(io.Discard, nil)})
	vl.Start()
	defer vl.Close(ctx)

	var metricsEnabled atomic.Bool
	if err := applyIntegrations(ctx, st, vm, vl, &metricsEnabled, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatal(err)
	}
	if !metricsEnabled.Load() {
		t.Fatal("applyIntegrations did not cache metrics_enabled")
	}
	vm.OnResult(ctx, &store.Result{Status: "ok", StartedAt: "2026-09-14T10:00:00.000Z"}, vmpush.Meta{})
	select {
	case p := <-hits:
		if p != "/api/v1/import/prometheus" {
			t.Fatalf("path = %s", p)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("applyIntegrations did not enable the VM writer")
	}
}

func TestApplyAuthReadsTheSettingsSection(t *testing.T) {
	st := newTestSettings(t)
	ctx := context.Background()
	if err := st.Set(ctx, settings.KeyAuthMode, settings.AuthModeForward); err != nil {
		t.Fatal(err)
	}
	if err := st.Set(ctx, settings.KeyAuthTrustedProxies, []string{"10.0.0.0/8"}); err != nil {
		t.Fatal(err)
	}
	m := auth.New(slog.Default(), noTokens{}, time.Now)
	if err := applyAuth(ctx, st, m, slog.Default()); err != nil {
		t.Fatal(err)
	}

	// auth.Middleware has no way to report its own mode, so exercise the
	// applied config through Identify: a trusted-proxy forward-auth
	// request only succeeds once the forward_auth section (mode, trusted
	// proxies, default headers) has actually been configured.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/targets", nil)
	req.RemoteAddr = "10.1.2.3:5555"
	req.Header.Set("Remote-User", "alice")
	id, err := m.Identify(req)
	if err != nil || id.User != "alice" || id.Mode != settings.AuthModeForward {
		t.Fatalf("identify = %+v, %v; want forward-auth mode applied from settings", id, err)
	}
}

func TestApplyAuthKeepsPreviousConfigOnBadCIDR(t *testing.T) {
	st := newTestSettings(t)
	ctx := context.Background()
	m := auth.New(slog.Default(), noTokens{}, time.Now)
	if err := applyAuth(ctx, st, m, slog.Default()); err != nil {
		t.Fatal(err)
	}
	if err := st.Set(ctx, settings.KeyAuthTrustedProxies, []string{"garbage"}); err != nil {
		t.Fatal(err)
	}
	if err := applyAuth(ctx, st, m, slog.Default()); err == nil {
		t.Fatal("want an error for an unparseable CIDR")
	}

	id, err := m.Identify(httptest.NewRequest(http.MethodGet, "/api/v1/targets", nil))
	if err != nil || id.Mode != settings.AuthModeOpen || !id.IsAdmin {
		t.Fatalf("identify = %+v, %v; want the instance to keep serving under the previous (open) config", id, err)
	}
}

func TestWatchSettingsAppliesAuthChanges(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "watch-auth.db"))
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
	reg := engine.NewRegistry()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	servers := ookla.NewServerList("speedtest", time.Hour)
	sch := scheduler.New(scheduler.Config{Store: db, Runner: runner.New(runner.Config{Store: db, Registry: reg, Hub: sse.NewHub(), Logger: logger}), Logger: logger})
	vm := vmpush.New(vmpush.Config{})
	vm.Start()
	defer vm.Close(context.Background())
	vl := vlpush.New(vlpush.Config{Next: slog.NewJSONHandler(io.Discard, nil)})
	vl.Start()
	defer vl.Close(context.Background())
	nt := notify.New(notify.Config{Store: db, Logger: logger})
	nt.Start()
	defer nt.Close(context.Background())
	am := newAuthAdapter(auth.New(logger, noTokens{}, time.Now))

	ir := iperf3list.New(iperf3list.Config{Store: db, Logger: logger})

	changes, unsubscribe := st.Subscribe()
	defer unsubscribe()
	var metricsEnabled atomic.Bool
	go watchSettings(ctx, st, changes, level, reg, servers, sch, vm, vl, nt, am, &metricsEnabled, ir, logger)

	if err := st.Set(ctx, settings.KeyAuthMode, settings.AuthModeToken); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for am.Mode() != settings.AuthModeToken && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if am.Mode() != settings.AuthModeToken {
		t.Errorf("mode = %q, want %q", am.Mode(), settings.AuthModeToken)
	}
}

func TestWatchSettingsStopsOnContextCancel(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "watch-cancel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx, cancel := context.WithCancel(context.Background())
	st, err := settings.New(ctx, db)
	if err != nil {
		t.Fatal(err)
	}

	level := new(slog.LevelVar)
	reg := engine.NewRegistry()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	servers := ookla.NewServerList("speedtest", time.Hour)
	sch := scheduler.New(scheduler.Config{Store: db, Runner: runner.New(runner.Config{Store: db, Registry: reg, Hub: sse.NewHub(), Logger: logger}), Logger: logger})
	vm := vmpush.New(vmpush.Config{})
	vm.Start()
	defer vm.Close(context.Background())
	vl := vlpush.New(vlpush.Config{Next: slog.NewJSONHandler(io.Discard, nil)})
	vl.Start()
	defer vl.Close(context.Background())
	nt := notify.New(notify.Config{Store: db, Logger: logger})
	nt.Start()
	defer nt.Close(context.Background())
	am := newAuthAdapter(auth.New(logger, noTokens{}, time.Now))

	ir := iperf3list.New(iperf3list.Config{Store: db, Logger: logger})

	changes, unsubscribe := st.Subscribe()
	defer unsubscribe()
	var metricsEnabled atomic.Bool
	done := make(chan struct{})
	go func() {
		defer close(done)
		watchSettings(ctx, st, changes, level, reg, servers, sch, vm, vl, nt, am, &metricsEnabled, ir, logger)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("watchSettings did not stop after context cancel")
	}
}
