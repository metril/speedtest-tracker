package iperf3list

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/metril/speedtest-tracker/internal/store"
)

// timeFormat is the timestamp format store.ReplaceIperf3Servers stamps
// fetched_at with (strftime('%Y-%m-%dT%H:%M:%fZ','now')).
const timeFormat = "2006-01-02T15:04:05.000Z"

// defaultInterval is how often Run refreshes the cache, and the staleness
// threshold that triggers a refresh on startup.
const defaultInterval = 24 * time.Hour

// Config configures a Refresher.
type Config struct {
	Store      *store.Store
	HTTPClient *http.Client
	URL        string
	Logger     *slog.Logger
	Interval   time.Duration // default 24h
	Now        func() time.Time
}

// Refresher keeps the cached public iperf3 server list up to date: on
// Run, it refreshes immediately if the cache is empty or older than
// Interval, then refreshes again every Interval until ctx is canceled.
// RefreshNow additionally lets a caller (the API's POST
// /iperf3/servers/refresh handler) trigger an immediate, synchronous
// refresh using its own request context.
type Refresher struct {
	cfg Config
}

// New returns a ready-to-run Refresher, applying defaults for any
// zero-valued Config field.
func New(cfg Config) *Refresher {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: fetchTimeout}
	}
	if cfg.Interval <= 0 {
		cfg.Interval = defaultInterval
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Refresher{cfg: cfg}
}

// RefreshNow fetches the upstream list and replaces the cache, unless the
// fetch yields zero rows — in which case the existing table is left alone
// (a thin/broken upstream response must never wipe a good cache). It
// returns the resulting fetched_at and total server count either way.
func (r *Refresher) RefreshNow(ctx context.Context) (time.Time, int, error) {
	servers, err := Fetch(ctx, r.cfg.HTTPClient, r.cfg.URL)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("iperf3list: fetch: %w", err)
	}
	if len(servers) > 0 {
		if err := r.cfg.Store.ReplaceIperf3Servers(ctx, servers); err != nil {
			return time.Time{}, 0, fmt.Errorf("iperf3list: replace: %w", err)
		}
	}

	fetchedAtStr, err := r.cfg.Store.Iperf3ServersFetchedAt(ctx)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("iperf3list: read fetched_at: %w", err)
	}
	var fetchedAt time.Time
	if fetchedAtStr != "" {
		fetchedAt, _ = time.Parse(timeFormat, fetchedAtStr)
	}

	count, err := r.cfg.Store.CountIperf3Servers(ctx)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("iperf3list: count: %w", err)
	}
	return fetchedAt, count, nil
}

// Run refreshes the cache on startup (if empty or stale) and then on
// every tick of Interval, until ctx is done. Errors are logged at warn
// and never crash the process — a failed refresh just leaves the
// previous cache in place until the next attempt.
func (r *Refresher) Run(ctx context.Context) {
	if r.shouldRefreshOnStart(ctx) {
		r.refreshAndLog(ctx)
	}

	ticker := time.NewTicker(r.cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.refreshAndLog(ctx)
		}
	}
}

func (r *Refresher) shouldRefreshOnStart(ctx context.Context) bool {
	fetchedAtStr, err := r.cfg.Store.Iperf3ServersFetchedAt(ctx)
	if err != nil {
		r.cfg.Logger.Warn("iperf3list: read fetched_at for startup check", "error", err)
		return true
	}
	if fetchedAtStr == "" {
		return true
	}
	fetchedAt, err := time.Parse(timeFormat, fetchedAtStr)
	if err != nil {
		return true
	}
	return r.cfg.Now().Sub(fetchedAt) >= r.cfg.Interval
}

func (r *Refresher) refreshAndLog(ctx context.Context) {
	if _, _, err := r.RefreshNow(ctx); err != nil {
		r.cfg.Logger.Warn("iperf3list: refresh failed", "error", err)
	}
}
