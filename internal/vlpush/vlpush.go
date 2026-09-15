// Package vlpush mirrors slog records to stdout and batches them as JSON
// lines to VictoriaLogs. Logging must never wait on the network, so
// Handle only renders the line and does a non-blocking send: when the
// queue is full the line is dropped and counted.
package vlpush

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/metril/speedtest-tracker/internal/settings"
)

// Config configures a Handler. Zero values get sane defaults in New.
type Config struct {
	Next          slog.Handler
	Client        *http.Client
	QueueSize     int
	BatchSize     int
	FlushInterval time.Duration
	App           string
}

// shared holds the mutable config, queue and worker state common to a
// Handler and every clone produced by WithAttrs/WithGroup.
type shared struct {
	mu           sync.Mutex // guards enabled/url/auth/streamFields only; never held across I/O
	enabled      bool
	url          string
	auth         settings.ExportAuth
	streamFields map[string]string

	lines chan []byte // handoff from Handle to the worker

	stop     chan struct{}
	stopOnce sync.Once
	done     chan struct{}

	// reqCtx bounds in-flight and pending flush POSTs during normal
	// operation. It is cancelled only after done closes (see Close), so it
	// never races the final drainAndFlush, which gets its own bounded
	// context (finalCtx) instead.
	reqCtx    context.Context
	reqCancel context.CancelFunc

	// finalCtx bounds the last drainAndFlush POST on shutdown. It is set
	// once by Close before stop is closed, so the worker goroutine observes
	// it safely via the happens-before edge on the stop channel close/recv.
	finalCtx context.Context

	dropped atomic.Int64

	client *http.Client
	app    string
	batch  int
	flush  time.Duration
	errLog *slog.Logger

	lastErrMu   sync.Mutex
	lastErrLogT time.Time
}

// Handler implements slog.Handler, mirroring every record to Next and
// batching it for shipment to VictoriaLogs.
type Handler struct {
	cfg    Config
	attrs  []slog.Attr
	groups []string

	shared *shared // config, queue and worker, shared by every WithAttrs clone
}

// New builds a Handler with defaults applied for any zero Config field.
func New(cfg Config) *Handler {
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 4096
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 100
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = 2 * time.Second
	}
	if cfg.Client == nil {
		cfg.Client = &http.Client{Timeout: 10 * time.Second}
	}
	if cfg.App == "" {
		cfg.App = "speedtest-tracker"
	}
	if cfg.Next == nil {
		cfg.Next = slog.NewJSONHandler(os.Stdout, nil)
	}
	reqCtx, reqCancel := context.WithCancel(context.Background())
	return &Handler{
		cfg: cfg,
		shared: &shared{
			lines:     make(chan []byte, cfg.QueueSize),
			stop:      make(chan struct{}),
			done:      make(chan struct{}),
			reqCtx:    reqCtx,
			reqCancel: reqCancel,
			client:    cfg.Client,
			app:       cfg.App,
			batch:     cfg.BatchSize,
			flush:     cfg.FlushInterval,
			errLog:    slog.New(cfg.Next),
		},
	}
}

// Configure hot-reloads the shipping target and labels. Safe to call
// concurrently with Handle and the worker loop.
func (h *Handler) Configure(enabled bool, u string, auth settings.ExportAuth, streamFields map[string]string) {
	cloned := make(map[string]string, len(streamFields))
	for k, v := range streamFields {
		cloned[k] = v
	}
	h.shared.mu.Lock()
	h.shared.enabled = enabled
	h.shared.url = u
	h.shared.auth = auth
	h.shared.streamFields = cloned
	h.shared.mu.Unlock()
}

// Start launches the worker goroutine. Call once.
func (h *Handler) Start() {
	go h.shared.run()
}

// Dropped returns the count of log lines dropped because the queue was
// full or a flush failed.
func (h *Handler) Dropped() int64 {
	return h.shared.dropped.Load()
}

// Close stops the worker and waits for it to drain, or ctx to expire. The
// final flush uses ctx as its own deadline, independent of reqCtx, which is
// cancelled only once the worker has actually finished (or leaked, if ctx
// expires first) to free its resources.
func (h *Handler) Close(ctx context.Context) error {
	h.shared.stopOnce.Do(func() {
		h.shared.finalCtx = ctx
		close(h.shared.stop)
	})
	go func() {
		<-h.shared.done
		h.shared.reqCancel()
	}()
	select {
	case <-h.shared.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Enabled delegates to the wrapped handler.
func (h *Handler) Enabled(ctx context.Context, lvl slog.Level) bool {
	return h.cfg.Next.Enabled(ctx, lvl)
}

// WithAttrs returns a shallow clone with the attrs appended, sharing the
// same queue/worker as h.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := *h
	clone.cfg.Next = h.cfg.Next.WithAttrs(attrs)
	clone.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &clone
}

// WithGroup returns a shallow clone with the group appended, sharing the
// same queue/worker as h.
func (h *Handler) WithGroup(name string) slog.Handler {
	clone := *h
	clone.cfg.Next = h.cfg.Next.WithGroup(name)
	clone.groups = append(append([]string{}, h.groups...), name)
	return &clone
}

// Handle mirrors rec to the wrapped handler (the source of truth for
// stdout output) and, if shipping is enabled, renders it as a JSON line
// and hands it to the worker without blocking.
func (h *Handler) Handle(ctx context.Context, rec slog.Record) error {
	if err := h.cfg.Next.Handle(ctx, rec); err != nil {
		return err
	}

	h.shared.mu.Lock()
	enabled, u, streamFields := h.shared.enabled, h.shared.url, h.shared.streamFields
	h.shared.mu.Unlock()
	if !enabled || u == "" {
		return nil
	}

	line := map[string]any{
		"_msg":  rec.Message,
		"_time": rec.Time.UTC().Format(time.RFC3339Nano),
		"level": rec.Level.String(),
		"app":   h.shared.app,
	}
	for k, v := range streamFields {
		line[k] = v
	}

	addAttr := func(groups []string, a slog.Attr) {
		key := a.Key
		if len(groups) > 0 {
			key = strings.Join(groups, ".") + "." + key
		}
		line[key] = jsonSafe(a.Value.Resolve().Any())
	}
	for _, a := range h.attrs {
		addAttr(h.groups, a)
	}
	rec.Attrs(func(a slog.Attr) bool {
		addAttr(h.groups, a)
		return true
	})

	b, err := json.Marshal(line)
	if err != nil {
		return nil
	}

	select {
	case h.shared.lines <- b:
	default:
		h.shared.dropped.Add(1)
	}
	return nil
}

// jsonSafe returns v if json.Marshal can encode it, otherwise its
// fmt.Sprint representation.
func jsonSafe(v any) any {
	if _, err := json.Marshal(v); err != nil {
		return fmt.Sprint(v)
	}
	return v
}

func (s *shared) run() {
	defer close(s.done)
	ticker := time.NewTicker(s.flush)
	defer ticker.Stop()

	buf := make([][]byte, 0, s.batch)
	for {
		select {
		case <-s.stop:
			s.drainAndFlush(buf)
			return
		default:
		}
		select {
		case b := <-s.lines:
			buf = append(buf, b)
			if len(buf) >= s.batch {
				s.doFlush(s.reqCtx, buf)
				buf = buf[:0]
			}
		case <-ticker.C:
			if len(buf) > 0 {
				s.doFlush(s.reqCtx, buf)
				buf = buf[:0]
			}
		case <-s.stop:
			s.drainAndFlush(buf)
			return
		}
	}
}

// drainAndFlush pulls any lines already queued, appends them to buf, and
// flushes once before the worker exits, using finalCtx (set by Close)
// rather than reqCtx so the last flush is not cancelled the instant Close
// is called. The flush is additionally capped at 10s regardless of
// finalCtx's own deadline (even none at all, e.g. context.Background()), so
// a caller that forgets to bound Close's ctx can never hang on a stalled
// endpoint.
func (s *shared) drainAndFlush(buf [][]byte) {
	for {
		select {
		case b := <-s.lines:
			buf = append(buf, b)
		default:
			if len(buf) > 0 {
				parent := s.finalCtx
				if parent == nil {
					parent = context.Background()
				}
				ctx, cancel := context.WithTimeout(parent, 10*time.Second)
				defer cancel()
				s.doFlush(ctx, buf)
			}
			return
		}
	}
}

// doFlush ships buf to VictoriaLogs using ctx to bound the request. On
// failure the batch is dropped and counted; errors are logged to Next at
// most once per 30s so a VL outage cannot become a log storm.
func (s *shared) doFlush(ctx context.Context, buf [][]byte) {
	s.mu.Lock()
	u, auth := s.url, s.auth
	streamFields := s.streamFields
	s.mu.Unlock()

	if err := s.post(ctx, u, auth, streamFields, buf); err != nil {
		s.dropped.Add(int64(len(buf)))
		s.lastErrMu.Lock()
		shouldLog := time.Since(s.lastErrLogT) >= 30*time.Second
		if shouldLog {
			s.lastErrLogT = time.Now()
		}
		s.lastErrMu.Unlock()
		if shouldLog {
			s.errLog.Error("vlpush: flush failed", "error", err)
		}
	}
}

func (s *shared) post(ctx context.Context, u string, auth settings.ExportAuth, streamFields map[string]string, buf [][]byte) error {
	body := bytes.Join(buf, []byte("\n"))
	body = append(body, '\n')

	keys := make([]string, 0, len(streamFields)+2)
	keys = append(keys, "app", "level")
	for k := range streamFields {
		keys = append(keys, k)
	}
	sort.Strings(keys[2:])

	q := url.Values{}
	q.Set("_stream_fields", strings.Join(keys, ","))
	q.Set("_msg_field", "_msg")
	q.Set("_time_field", "_time")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(u, "/")+"/insert/jsonline?"+q.Encode(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/stream+json")
	auth.Apply(req)
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("vlpush: unexpected status %d", resp.StatusCode)
	}
	return nil
}
