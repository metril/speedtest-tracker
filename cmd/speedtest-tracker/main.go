// Command speedtest-tracker serves the API and the embedded web UI.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/metril/speedtest-tracker/internal/api"
	"github.com/metril/speedtest-tracker/internal/config"
	"github.com/metril/speedtest-tracker/internal/engine"
	"github.com/metril/speedtest-tracker/internal/engine/ookla"
	"github.com/metril/speedtest-tracker/internal/runner"
	"github.com/metril/speedtest-tracker/internal/scheduler"
	"github.com/metril/speedtest-tracker/internal/settings"
	"github.com/metril/speedtest-tracker/internal/sse"
	"github.com/metril/speedtest-tracker/internal/store"
	"github.com/metril/speedtest-tracker/internal/web"
)

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "-healthcheck" {
		os.Exit(healthcheckMain(config.Load().Listen))
	}
	if len(os.Args) > 1 && os.Args[1] == "run" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		code := runCmd(ctx, os.Args[2:], os.Stdout, os.Stderr)
		stop()
		os.Exit(code)
	}

	level := new(slog.LevelVar)
	level.Set(slog.LevelInfo)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, logger, level); err != nil {
		logger.Error("fatal", "error", err)
		os.Exit(1)
	}
}

// healthcheckMain runs the -healthcheck probe against the given ST_LISTEN
// address and returns a process exit code: 0 if the server answered
// /healthz with 200, 1 otherwise. It exists so the Docker HEALTHCHECK
// needs no curl/wget in the runtime image.
func healthcheckMain(listen string) int {
	if err := healthcheck(listen); err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	return 0
}

func healthcheck(listen string) error {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return fmt.Errorf("parse listen address %q: %w", listen, err)
	}
	if host == "" {
		host = "127.0.0.1"
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://%s:%s/healthz", host, port))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET /healthz: status %d", resp.StatusCode)
	}
	return nil
}

// parseLevel maps a settings log level string onto a slog.Level,
// defaulting to info for anything unrecognised.
func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// watchSettings applies live settings changes: general.log_level retunes
// the logger in place and any engines.* change rebuilds the engine
// registry and invalidates the Ookla server-list cache. It returns when
// ctx is done.
func watchSettings(ctx context.Context, st *settings.Store, level *slog.LevelVar,
	reg *engine.Registry, servers *ookla.ServerList, sch *scheduler.Scheduler, logger *slog.Logger) {
	changes, unsubscribe := st.Subscribe()
	defer unsubscribe()

	for {
		select {
		case <-ctx.Done():
			return
		case key, open := <-changes:
			if !open {
				return
			}
			switch {
			case key == settings.KeyLogLevel:
				g, err := st.General(ctx)
				if err != nil {
					logger.Error("reload general settings", "error", err)
					continue
				}
				level.Set(parseLevel(g.LogLevel))
				logger.Info("log level changed", "level", g.LogLevel)
			case strings.HasPrefix(key, "engines."):
				eng, err := st.Engines(ctx)
				if err != nil {
					logger.Error("reload engine settings", "error", err)
					continue
				}
				reg.Replace(buildEngines(eng))
				servers.Invalidate()
				logger.Info("engines rebuilt", "changed_key", key)
			case key == settings.KeyTimezone:
				// Schedules with no explicit timezone follow the general
				// setting, so a change means every cron entry is rebuilt.
				if err := sch.Reload(ctx); err != nil {
					logger.Error("reload schedules after timezone change", "error", err)
					continue
				}
				logger.Info("schedules reloaded after timezone change")
			}
		}
	}
}

func run(ctx context.Context, logger *slog.Logger, level *slog.LevelVar) error {
	cfg := config.Load()
	logger.Info("starting", "version", version, "db_path", cfg.DBPath, "listen", cfg.Listen)

	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()

	st, err := settings.New(ctx, db)
	if err != nil {
		return err
	}
	general, err := st.General(ctx)
	if err != nil {
		return err
	}
	level.Set(parseLevel(general.LogLevel))

	engineCfg, err := st.Engines(ctx)
	if err != nil {
		return err
	}
	reg := buildRegistry(engineCfg)
	servers := ookla.NewServerList(engineCfg.SpeedtestBin,
		time.Duration(engineCfg.ServerListTTLSeconds)*time.Second)

	hub := sse.NewHub()
	rn := runner.New(runner.Config{
		Store: db, Registry: reg, Hub: hub, Logger: logger,
	})
	rn.Start()

	sch := scheduler.New(scheduler.Config{Store: db, Runner: rn, Logger: logger})
	if err := sch.Reload(ctx); err != nil {
		return err
	}

	watchCtx, stopWatch := context.WithCancel(context.Background())
	defer stopWatch()
	go watchSettings(watchCtx, st, level, reg, servers, sch, logger)

	srv := &http.Server{
		Addr: cfg.Listen,
		Handler: api.New(api.Deps{
			Pinger:          db,
			Logger:          logger,
			UI:              web.Handler(),
			Hub:             hub,
			Store:           db,
			Registry:        reg,
			Runner:          rn,
			ServerList:      servers,
			ReloadSchedules: sch.Reload,
		}),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		logger.Info("shutting down")
		// Order: stop accepting HTTP, drain in-flight tests, then close
		// the database (deferred above).
		httpCtx, cancelHTTP := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelHTTP()
		if err := srv.Shutdown(httpCtx); err != nil {
			logger.Error("http shutdown", "error", err)
		}
		stopWatch()
		schedCtx, cancelSched := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelSched()
		if err := sch.Stop(schedCtx); err != nil {
			logger.Error("scheduler shutdown", "error", err)
		}
		runnerCtx, cancelRunner := context.WithTimeout(context.Background(), 70*time.Second)
		defer cancelRunner()
		if err := rn.Shutdown(runnerCtx); err != nil {
			logger.Error("runner shutdown", "error", err)
		}
		return <-errCh
	}
}
