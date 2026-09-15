package vmpush

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/metril/speedtest-tracker/internal/settings"
	"github.com/metril/speedtest-tracker/internal/store"
)

// Config configures a Writer. Zero values get sane defaults in New.
type Config struct {
	Client     *http.Client
	Logger     *slog.Logger
	RingSize   int
	MaxBackoff time.Duration
	Now        func() time.Time
}

// Stats are the running counters exposed for the /metrics endpoint.
type Stats struct {
	Pushed  int64
	Failed  int64
	Dropped int64
	Queued  int64
}

// Writer owns one goroutine that drains a bounded ring of batches and
// POSTs them to VictoriaMetrics. OnResult only formats the payload and
// hands it over, so a stalled or unreachable VM never slows a test down.
type Writer struct {
	cfg Config

	mu      sync.Mutex // guards config only; never held across I/O
	enabled bool
	url     string
	auth    settings.ExportAuth
	extra   map[string]string

	enqueueMu sync.Mutex // serializes the drop-oldest ring dance in OnResult

	in       chan []byte // handoff from OnResult to the worker
	stop     chan struct{}
	stopOnce sync.Once
	done     chan struct{}

	// flushCtx bounds delivery of batches drained from in on shutdown. It
	// is set once by Close before stop is closed, observed safely by the
	// worker via the happens-before edge on that channel close/recv.
	flushCtx context.Context

	pushed, failed, dropped atomic.Int64
}

// New builds a Writer with defaults applied for any zero Config field.
func New(cfg Config) *Writer {
	if cfg.RingSize <= 0 {
		cfg.RingSize = 200
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = 60 * time.Second
	}
	if cfg.Client == nil {
		cfg.Client = &http.Client{Timeout: 15 * time.Second}
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Writer{
		cfg:  cfg,
		in:   make(chan []byte, cfg.RingSize),
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
}

// Configure hot-reloads the target and labels. Safe to call concurrently
// with OnResult and the worker loop.
func (w *Writer) Configure(enabled bool, url string, auth settings.ExportAuth, extra map[string]string) {
	cloned := make(map[string]string, len(extra))
	for k, v := range extra {
		cloned[k] = v
	}
	w.mu.Lock()
	w.enabled = enabled
	w.url = url
	w.auth = auth
	w.extra = cloned
	w.mu.Unlock()
}

// Start launches the worker goroutine. Call once.
func (w *Writer) Start() {
	go w.run()
}

// OnResult formats res and hands it to the worker without blocking. When
// the ring is full it drops the oldest queued batch, counting it, so a
// burst never backs up the caller.
func (w *Writer) OnResult(ctx context.Context, res *store.Result, meta Meta) {
	w.mu.Lock()
	enabled, url, extra := w.enabled, w.url, w.extra
	w.mu.Unlock()
	if !enabled || url == "" {
		return
	}
	b := Format(res, meta, extra)

	// The drop-oldest dance below is three separate non-atomic steps; guard
	// it with a mutex so concurrent callers can't double-drop or steal each
	// other's batch. Never held across I/O.
	w.enqueueMu.Lock()
	defer w.enqueueMu.Unlock()
	select {
	case w.in <- b:
		return
	default:
	}
	select {
	case <-w.in:
		w.dropped.Add(1)
	default:
	}
	select {
	case w.in <- b:
	default:
		w.dropped.Add(1)
	}
}

// Stats returns a snapshot of the running counters.
func (w *Writer) Stats() Stats {
	return Stats{
		Pushed:  w.pushed.Load(),
		Failed:  w.failed.Load(),
		Dropped: w.dropped.Load(),
		Queued:  int64(len(w.in)),
	}
}

// Close stops the worker and waits for it to drain, or ctx to expire. ctx
// also bounds delivery of whatever is still queued in w.in at shutdown, so
// that work is attempted (per Close's doc) instead of silently discarded.
func (w *Writer) Close(ctx context.Context) error {
	w.stopOnce.Do(func() {
		w.flushCtx = ctx
		close(w.stop)
	})
	select {
	case <-w.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *Writer) run() {
	defer close(w.done)
	for {
		select {
		case b := <-w.in:
			w.deliver(b)
		case <-w.stop:
			w.drainAndFlush()
			return
		}
	}
}

// drainAndFlush pulls every batch still queued in w.in and attempts
// delivery of each within flushCtx's deadline, so a shutdown drains the
// queue instead of discarding it outright. The 10s cap applies regardless
// of flushCtx's own deadline (even none at all, e.g. context.Background()),
// so a caller that forgets to bound Close's ctx can never turn a dead
// endpoint into a Close that hangs forever.
func (w *Writer) drainAndFlush() {
	parent := w.flushCtx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	for {
		select {
		case b := <-w.in:
			w.deliverBounded(ctx, b)
		default:
			return
		}
	}
}

// deliverBounded retries a batch with exponential backoff until it
// succeeds, a non-retryable 4xx is returned, the integration is disabled,
// or ctx expires — at which point the batch is abandoned and counted as
// dropped rather than failed, since it was never conclusively rejected.
func (w *Writer) deliverBounded(ctx context.Context, b []byte) {
	backoff := 200 * time.Millisecond
	for {
		w.mu.Lock()
		enabled, url, auth := w.enabled, w.url, w.auth
		w.mu.Unlock()
		if !enabled {
			w.dropped.Add(1)
			return
		}

		status, err := w.post(ctx, url, auth, b)
		if err == nil && status < 300 {
			w.pushed.Add(1)
			return
		}
		w.failed.Add(1)
		if err == nil && status >= 400 && status < 500 && status != http.StatusTooManyRequests {
			return
		}

		select {
		case <-ctx.Done():
			w.dropped.Add(1)
			return
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > w.cfg.MaxBackoff {
			backoff = w.cfg.MaxBackoff
		}
	}
}

// deliver retries a batch with exponential backoff until it succeeds, a
// non-retryable 4xx is returned, the integration is disabled, or Close is
// requested.
func (w *Writer) deliver(b []byte) {
	backoff := 200 * time.Millisecond
	for {
		w.mu.Lock()
		enabled, url, auth := w.enabled, w.url, w.auth
		w.mu.Unlock()
		if !enabled {
			// Turned off mid-retry: abandon the batch, counted once here
			// rather than once per attempt already spent on it.
			w.failed.Add(1)
			return
		}

		status, err := w.post(context.Background(), url, auth, b)
		if err == nil && status < 300 {
			w.pushed.Add(1)
			return
		}
		w.failed.Add(1)
		if err == nil && status >= 400 && status < 500 && status != http.StatusTooManyRequests {
			// Bad request: retrying will never succeed. Drop it.
			return
		}

		select {
		case <-w.stop:
			return
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > w.cfg.MaxBackoff {
			backoff = w.cfg.MaxBackoff
		}
	}
}

func (w *Writer) post(parent context.Context, url string, auth settings.ExportAuth, b []byte) (int, error) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(url, "/")+"/api/v1/import/prometheus", bytes.NewReader(b))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "text/plain")
	auth.Apply(req)
	resp, err := w.cfg.Client.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()
	if resp.StatusCode >= 300 {
		w.cfg.Logger.Warn("vmpush: push failed", "status", resp.StatusCode)
	}
	return resp.StatusCode, nil
}
