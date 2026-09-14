// Notifier evaluates completed results and delivers alert/recovery
// notifications asynchronously, so the runner's lane goroutines are never
// blocked on a slow or unreachable channel.
package notify

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/metril/speedtest-tracker/internal/settings"
	"github.com/metril/speedtest-tracker/internal/store"
)

// lastFiredLayout is the timestamp layout notification_state.last_fired_at
// is stored and parsed in.
const lastFiredLayout = "2006-01-02T15:04:05.000Z"

// Config wires a Notifier to its dependencies.
type Config struct {
	Store    *store.Store
	Client   *http.Client
	Logger   *slog.Logger
	QueueCap int
	Now      func() time.Time
}

// Meta is the run context OnResult receives alongside a result.
type Meta struct {
	ScheduleName string
}

// Stats is a point-in-time snapshot of the notifier's counters.
type Stats struct {
	Sent       int64
	Failed     int64
	Dropped    int64
	Suppressed int64
	Queued     int
}

// Notifier evaluates results and delivers notifications. OnResult is
// called on the runner's lane goroutine and must never block, so it only
// does a non-blocking send onto queue; everything else — the state reads
// and writes and all HTTP — happens on the worker goroutine started by
// Start.
type Notifier struct {
	cfg   Config
	queue chan *store.Result

	mu   sync.Mutex // guards conf/loc only; never held across I/O
	conf settings.Notifications
	loc  *time.Location

	sent, failed, dropped, suppressed atomic.Int64

	startOnce sync.Once
	stop      chan struct{}
	done      chan struct{}
}

// New builds a Notifier. It does not start the worker goroutine; call
// Start for that.
func New(cfg Config) *Notifier {
	if cfg.QueueCap <= 0 {
		cfg.QueueCap = 256
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Client == nil {
		cfg.Client = &http.Client{Timeout: 10 * time.Second}
	}
	return &Notifier{
		cfg:   cfg,
		queue: make(chan *store.Result, cfg.QueueCap),
		loc:   time.UTC,
		stop:  make(chan struct{}),
		done:  make(chan struct{}),
	}
}

// Configure replaces the active settings section and timezone. Safe to
// call at boot and on every subsequent settings change; never holds the
// mutex across I/O.
func (n *Notifier) Configure(cfg settings.Notifications, loc *time.Location) {
	if loc == nil {
		loc = time.UTC
	}
	n.mu.Lock()
	n.conf = cfg
	n.loc = loc
	n.mu.Unlock()
}

// snapshot returns a copy of the current settings and location. Every
// read path uses this instead of touching conf/loc directly, so the
// mutex is never held during a DB query or an HTTP call.
func (n *Notifier) snapshot() (settings.Notifications, *time.Location) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.conf, n.loc
}

// Start launches the worker goroutine that drains the queue and calls
// process. Safe to call more than once; only the first call has effect.
func (n *Notifier) Start() {
	n.startOnce.Do(func() {
		go func() {
			defer close(n.done)
			for {
				select {
				case res := <-n.queue:
					n.process(context.Background(), res)
				case <-n.stop:
					// Drain whatever is already queued rather than
					// dropping it: Close documents "waits to drain", and
					// OnResult may have queued a result concurrently with
					// Close being called.
					for {
						select {
						case res := <-n.queue:
							n.process(context.Background(), res)
						default:
							return
						}
					}
				}
			}
		}()
	})
}

// Close signals the worker to stop and waits for it to drain, or for ctx
// to be done, whichever comes first.
func (n *Notifier) Close(ctx context.Context) error {
	select {
	case <-n.stop:
	default:
		close(n.stop)
	}
	select {
	case <-n.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// OnResult implements runner.ResultSink-shaped delivery: it must never
// block the caller's lane goroutine. res is owned by the runner and may
// be mutated after this call returns, so it is copied before queueing.
func (n *Notifier) OnResult(ctx context.Context, res *store.Result, meta Meta) {
	cp := *res
	select {
	case n.queue <- &cp:
	default:
		// Queue full: drop the oldest so a burst of results cannot wedge
		// the lane worker, and count it.
		select {
		case <-n.queue:
			n.dropped.Add(1)
		default:
		}
		select {
		case n.queue <- &cp:
		default:
			n.dropped.Add(1)
		}
	}
}

// Stats returns a snapshot of the notifier's counters.
func (n *Notifier) Stats() Stats {
	return Stats{
		Sent:       n.sent.Load(),
		Failed:     n.failed.Load(),
		Dropped:    n.dropped.Load(),
		Suppressed: n.suppressed.Load(),
		Queued:     len(n.queue),
	}
}

// process is the synchronous body of result handling: evaluate against
// thresholds, apply cooldown and quiet-hours rules, persist state and
// deliver. It is exported to tests within the package so cooldown and
// quiet-hours behavior can be driven without test sleeps.
func (n *Notifier) process(ctx context.Context, res *store.Result) {
	conf, loc := n.snapshot()
	if !conf.Enabled || res.TargetID == nil {
		return
	}

	tgt, err := n.cfg.Store.GetTarget(ctx, *res.TargetID)
	if err != nil {
		// A deleted target means nothing left to notify about.
		n.cfg.Logger.Debug("notify: target not found", "target_id", *res.TargetID, "err", err)
		return
	}

	perTarget, nulled, err := ParseThresholds(tgt.Thresholds)
	if err != nil {
		// A corrupt threshold document must not stop later results.
		n.cfg.Logger.Warn("notify: bad thresholds", "target", tgt.Name, "target_id", tgt.ID, "err", err)
		return
	}

	th := Merge(conf.DefaultThresholds, perTarget, nulled)
	evals := Evaluate(res, th)

	now := n.cfg.Now()
	quiet := quietNow(now, conf.QuietHoursStart, conf.QuietHoursEnd, loc)
	cooldown := time.Duration(conf.CooldownMinutes) * time.Minute

	for _, e := range evals {
		n.handleEval(ctx, conf, tgt, res, e, now, quiet, cooldown)
	}
	n.clearStaleState(ctx, tgt, evals)
}

// allMetrics lists every metric Evaluate can produce.
var allMetrics = []Metric{MetricDownload, MetricUpload, MetricPing, MetricJitter, MetricLoss, MetricFailure}

// clearStaleState clears any stored notify state for a metric that
// Evaluate did not emit this round, e.g. because its threshold was just
// disabled (set to null) or unset entirely. Without this, a metric that
// was firing when its threshold is removed would never come back through
// handleEval (its only caller of ClearNotifyState) to recover: the state
// row would stay firing=1 forever, and a later re-enable could then fire a
// stale "recovered" for a breach nobody currently observes. Cleared
// silently: no recovery notification is delivered, since the metric is no
// longer being evaluated at all.
func (n *Notifier) clearStaleState(ctx context.Context, tgt *store.Target, evals []Eval) {
	present := make(map[Metric]bool, len(evals))
	for _, e := range evals {
		present[e.Metric] = true
	}
	for _, m := range allMetrics {
		if present[m] {
			continue
		}
		metric := string(m)
		if _, ok, err := n.cfg.Store.GetNotifyState(ctx, tgt.ID, metric); err != nil || !ok {
			if err != nil {
				n.cfg.Logger.Warn("notify: get stale state", "target", tgt.Name, "metric", metric, "err", err)
			}
			continue
		}
		if err := n.cfg.Store.ClearNotifyState(ctx, tgt.ID, metric); err != nil {
			n.cfg.Logger.Warn("notify: clear stale state", "target", tgt.Name, "metric", metric, "err", err)
		}
	}
}

func (n *Notifier) handleEval(ctx context.Context, conf settings.Notifications, tgt *store.Target, res *store.Result, e Eval, now time.Time, quiet bool, cooldown time.Duration) {
	metric := string(e.Metric)
	st, ok, err := n.cfg.Store.GetNotifyState(ctx, tgt.ID, metric)
	if err != nil {
		n.cfg.Logger.Warn("notify: get state", "target", tgt.Name, "metric", metric, "err", err)
		return
	}

	if e.Breached {
		elapsed := !ok || cooldownElapsed(st.LastFiredAt, now, cooldown)
		if !elapsed {
			return
		}
		msg := BuildMessage("alert", tgt.Name, tgt.ID, res.ID, e, now, res.Error)
		n.fire(ctx, conf, tgt.ID, metric, now, quiet, msg, ok)
		return
	}

	// Not breached. A stored state with an empty LastFiredAt means the
	// alert was only ever recorded during quiet hours and never actually
	// delivered (see fire), so there is nothing to report as "recovered".
	if !ok || !st.Firing {
		return
	}
	delivered := st.LastFiredAt != ""
	send := conf.NotifyRecovery && delivered
	var msg Message
	if send {
		msg = BuildMessage("recovery", tgt.Name, tgt.ID, res.ID, e, now, "")
	}
	n.recover(ctx, conf, tgt.ID, metric, quiet, msg, send)
}

// cooldownElapsed reports whether enough time has passed since
// lastFiredAt (in the store's timestamp layout) for a new alert to fire.
// A parse failure is treated as "cooldown elapsed" so a corrupt or empty
// timestamp cannot wedge an alert shut forever.
func cooldownElapsed(lastFiredAt string, now time.Time, cooldown time.Duration) bool {
	last, err := time.Parse(lastFiredLayout, lastFiredAt)
	if err != nil {
		return true
	}
	return now.Sub(last) >= cooldown
}

// fire records the breach and, unless quiet hours suppress delivery,
// sends the alert. During quiet hours nothing is delivered and
// LastFiredAt is left unset (or, if a row already exists, left exactly as
// it was): setting it to "now" would both start the cooldown clock for an
// alert nobody received (delaying the first real alert by a full
// cooldown) and make a later recovery think something was actually sent.
// alreadyFiring is whether a state row already existed for this
// (target,metric) pair; when quiet and it did, the row (and whatever
// LastFiredAt it carries — set or still empty) is left untouched, so a
// burst of suppressed re-fires does not keep rewriting it.
func (n *Notifier) fire(ctx context.Context, conf settings.Notifications, targetID int64, metric string, now time.Time, quiet bool, msg Message, alreadyFiring bool) {
	if quiet {
		n.suppressed.Add(1)
		if alreadyFiring {
			return
		}
		if err := n.cfg.Store.SetNotifyState(ctx, store.NotifyState{
			TargetID: targetID, Metric: metric, Firing: true, LastFiredAt: "",
		}); err != nil {
			n.cfg.Logger.Warn("notify: set state", "target_id", targetID, "metric", metric, "err", err)
		}
		return
	}
	if err := n.cfg.Store.SetNotifyState(ctx, store.NotifyState{
		TargetID: targetID, Metric: metric, Firing: true,
		LastFiredAt: now.UTC().Format(lastFiredLayout),
	}); err != nil {
		n.cfg.Logger.Warn("notify: set state", "target_id", targetID, "metric", metric, "err", err)
		return
	}
	n.deliver(ctx, conf, msg)
}

// recover clears firing state (dropping the row rather than storing
// firing=0 keeps the table the size of the active alert set) and,
// unless quiet hours suppress it or recovery notices are disabled,
// delivers the recovery message.
func (n *Notifier) recover(ctx context.Context, conf settings.Notifications, targetID int64, metric string, quiet bool, msg Message, send bool) {
	if err := n.cfg.Store.ClearNotifyState(ctx, targetID, metric); err != nil {
		n.cfg.Logger.Warn("notify: clear state", "target_id", targetID, "metric", metric, "err", err)
		return
	}
	if !send {
		return
	}
	if quiet {
		n.suppressed.Add(1)
		return
	}
	n.deliver(ctx, conf, msg)
}

// deliver iterates the configured channels, skipping disabled ones. One
// bad channel never short-circuits the loop.
func (n *Notifier) deliver(ctx context.Context, conf settings.Notifications, msg Message) {
	for _, ch := range conf.Channels {
		if !ch.Enabled {
			continue
		}
		cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err := Deliver(cctx, n.cfg.Client, ch, msg)
		cancel()
		if err != nil {
			n.failed.Add(1)
			n.cfg.Logger.Warn("notify: deliver failed", "channel", ch.ID, "err", err)
			continue
		}
		n.sent.Add(1)
	}
}

// TestChannel validates ch and delivers a fixed probe message to it,
// ignoring conf.Enabled so the button works before the section is
// switched on.
func (n *Notifier) TestChannel(ctx context.Context, ch settings.Channel) error {
	if err := ValidateChannel(ch); err != nil {
		return err
	}
	msg := Message{Kind: "alert", Title: "speedtest-tracker test notification",
		Body: "this is a test notification from speedtest-tracker", At: n.cfg.Now()}
	return Deliver(ctx, n.cfg.Client, ch, msg)
}

// quietNow reports whether now falls inside the [start, end) quiet-hours
// window, both given as "HH:MM" in loc. Equal start and end means "no
// quiet hours" (not "always quiet"); empty start or end never suppresses.
// When start > end the window wraps midnight (e.g. 22:00-07:00), so the
// test becomes "at or after start, or before end".
func quietNow(now time.Time, start, end string, loc *time.Location) bool {
	if start == "" || end == "" || start == end {
		return false
	}
	s, err1 := time.ParseInLocation("15:04", start, loc)
	e, err2 := time.ParseInLocation("15:04", end, loc)
	if err1 != nil || err2 != nil {
		return false
	}
	in := now.In(loc)
	mins := in.Hour()*60 + in.Minute()
	smins := s.Hour()*60 + s.Minute()
	emins := e.Hour()*60 + e.Minute()
	if smins > emins {
		return mins >= smins || mins < emins
	}
	return mins >= smins && mins < emins
}
