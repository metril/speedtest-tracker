// Command speedtest-tracker serves the API and the embedded web UI.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/metril/speedtest-tracker/internal/api"
	"github.com/metril/speedtest-tracker/internal/config"
	"github.com/metril/speedtest-tracker/internal/settings"
	"github.com/metril/speedtest-tracker/internal/store"
	"github.com/metril/speedtest-tracker/internal/web"
)

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, logger); err != nil {
		logger.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger) error {
	cfg := config.Load()
	logger.Info("starting", "version", version, "db_path", cfg.DBPath, "listen", cfg.Listen)

	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()

	if _, err := settings.New(ctx, db); err != nil {
		return err
	}

	srv := &http.Server{
		Addr: cfg.Listen,
		Handler: api.New(api.Deps{
			Pinger: db,
			Logger: logger,
			UI:     web.Handler(),
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
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return <-errCh
	}
}
