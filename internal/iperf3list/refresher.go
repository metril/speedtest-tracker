package iperf3list

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/metril/speedtest-tracker/internal/store"
)

// ErrDisabled is returned by RefreshNow when URLFunc reports an empty URL
// (the iperf3_list_url setting has been cleared), meaning refreshing is
// deliberately disabled rather than misconfigured.
var ErrDisabled = errors.New("iperf3list: refresh disabled (iperf3_list_url is empty)")

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

	// URLFunc returns the current feed URL, read fresh on every refresh
	// (so a settings change takes effect without restarting the
	// process). An empty return disables refreshing: RefreshNow returns
	// ErrDisabled without fetching anything. Required; a nil URLFunc
	// behaves as always-disabled.
	URLFunc func() string

	Logger   *slog.Logger
	Interval time.Duration // default 24h
	Now      func() time.Time
}

// Refresher keeps the cached public iperf3 server list up to date: on
// Run, it refreshes immediately if the cache is empty or older than
// Interval, then refreshes again every Interval until ctx is canceled.
// RefreshNow additionally lets a caller (the API's POST
// /iperf3/servers/refresh handler) trigger an immediate, synchronous
// refresh using its own request context.
type Refresher struct {
	cfg Config

	mu             sync.Mutex
	loggedDisabled bool

	// kick lets a caller (main.go's settings watcher, on an engines.*
	// change) ask Run to re-evaluate right away instead of waiting for
	// the next Interval tick. Buffered 1 and only ever sent to
	// non-blockingly by Kick, so a kick is never lost while Run is
	// mid-refresh, but a burst of kicks still collapses to one
	// re-evaluation.
	kick chan struct{}
}

// New returns a ready-to-run Refresher, applying defaults for any
// zero-valued Config field.
func New(cfg Config) *Refresher {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: fetchTimeout, CheckRedirect: rejectCrossHostRedirect}
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
	if cfg.URLFunc == nil {
		cfg.URLFunc = func() string { return "" }
	}
	return &Refresher{cfg: cfg, kick: make(chan struct{}, 1)}
}

// Kick asks a running Run loop to re-evaluate (and, if enabled, refresh)
// right away instead of waiting for the next Interval tick — e.g. after an
// engines.iperf3_list_url settings change. Non-blocking: safe to call from
// any goroutine, including before Run has started (the kick is buffered
// and picked up once Run's select loop begins) or after ctx is done (the
// send just lands in the buffer and is never read).
func (r *Refresher) Kick() {
	select {
	case r.kick <- struct{}{}:
	default:
	}
}

// RefreshNow reads the current feed URL from URLFunc and, if set, fetches
// the upstream list and replaces the cache, unless the fetch yields zero
// rows — in which case the existing table is left alone (a thin/broken
// upstream response must never wipe a good cache). It returns the
// resulting fetched_at and total server count either way.
//
// When URLFunc reports an empty URL, RefreshNow does not fetch anything
// and returns ErrDisabled; an info line is logged the first time this is
// observed (and again if it flips back to disabled after being enabled),
// not on every call, so a long-disabled feed does not spam the log once
// per refresh tick.
func (r *Refresher) RefreshNow(ctx context.Context) (time.Time, int, error) {
	url := r.cfg.URLFunc()
	if url == "" {
		r.mu.Lock()
		alreadyLogged := r.loggedDisabled
		r.loggedDisabled = true
		r.mu.Unlock()
		if !alreadyLogged {
			r.cfg.Logger.Info("iperf3list: refresh disabled (iperf3_list_url is empty)")
		}
		return time.Time{}, 0, ErrDisabled
	}
	r.mu.Lock()
	r.loggedDisabled = false
	r.mu.Unlock()

	servers, err := Fetch(ctx, r.cfg.HTTPClient, url)
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
		case <-r.kick:
			r.refreshAndLog(ctx)
			// A kick shouldn't also bring forward the *next* tick, so a
			// disabled-then-kicked instance still only refreshes as
			// often as Interval once re-enabled without another kick.
			ticker.Reset(r.cfg.Interval)
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
		if errors.Is(err, ErrDisabled) {
			// Already logged (once) inside RefreshNow.
			return
		}
		r.cfg.Logger.Warn("iperf3list: refresh failed", "error", err)
	}
}
