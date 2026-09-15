// Package runner executes speed tests. It keeps one buffered queue and one
// worker goroutine per queue so targets in different queues can run at the
// same time while targets in the same queue never overlap.
package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/metril/speedtest-tracker/internal/engine"
	"github.com/metril/speedtest-tracker/internal/sse"
	"github.com/metril/speedtest-tracker/internal/store"
)

// Errors returned by Enqueue.
var (
	ErrQueueFull    = errors.New("runner: queue is full")
	ErrNoTargets    = errors.New("runner: no runnable targets")
	ErrShuttingDown = errors.New("runner: shutting down")
)

// Publisher is the subset of *sse.Hub the runner needs.
type Publisher interface {
	Publish(sse.Event)
	Marshal(eventType string, payload any) sse.Event
}

// Config configures a Runner. Zero values get sane defaults.
type Config struct {
	Store       *store.Store
	Registry    *engine.Registry
	Hub         Publisher
	Sink        ResultSink // nil => no-op
	Logger      *slog.Logger
	QueueCap    int           // per-queue channel capacity, default 32
	Grace       time.Duration // shutdown grace period, default 60s
	TestTimeout time.Duration // per-target timeout, default 10m
	Now         func() time.Time
}

// ResultMeta is the run context a sink needs but the result row does not
// carry: which run and schedule produced it, and on which queue.
type ResultMeta struct {
	RunID        int64
	Trigger      string
	ScheduleID   *int64
	ScheduleName string
	QueueName    string
}

// ResultSink is notified once per persisted result. Implementations MUST
// return promptly and never block: they run on the queue worker goroutine,
// so a slow sink delays the next test. Anything that talks to the network
// hands the work to its own goroutine.
type ResultSink interface {
	OnResult(ctx context.Context, res *store.Result, meta ResultMeta)
}

// SinkFunc adapts a function to ResultSink.
type SinkFunc func(context.Context, *store.Result, ResultMeta)

// OnResult implements ResultSink.
func (f SinkFunc) OnResult(ctx context.Context, res *store.Result, m ResultMeta) { f(ctx, res, m) }

// Sinks fans one result out to every sink. A panicking or slow sink must
// not take the runner down or skip its siblings, so each call is
// recovered individually.
type Sinks []ResultSink

// OnResult implements ResultSink.
func (s Sinks) OnResult(ctx context.Context, res *store.Result, m ResultMeta) {
	for _, sink := range s {
		func() {
			defer func() {
				if p := recover(); p != nil {
					slog.Default().Error("result sink panicked", "panic", p)
				}
			}()
			sink.OnResult(ctx, res, m)
		}()
	}
}

// RunRequest asks for one run over the given targets. Snapshots optionally
// maps a target id to an exact options document to replay (re-execute)
// instead of the target's current live options; a target absent from the
// map runs with its live options as usual.
type RunRequest struct {
	Trigger    string
	ScheduleID *int64
	TargetIDs  []int64
	Snapshots  map[int64]json.RawMessage
}

// ProgressEvent is the payload of the SSE progress event. ResultID stays 0
// while the test runs — the row is written only on completion — so the UI
// keys live state on TargetID+RunID and picks up the id from the result
// event.
type ProgressEvent struct {
	RunID    int64  `json:"run_id"`
	ResultID int64  `json:"result_id"`
	TargetID int64  `json:"target_id"`
	Engine   string `json:"engine"`
	engine.Progress
}

// progressInterval caps progress events at 10 Hz per result.
const progressInterval = 100 * time.Millisecond

// job is one queue's share of a run. queueID is what routes it to the
// right worker (stable identity: a queue rename must not change which
// channel a job lands in); queueName is carried along only for display
// (ResultMeta, QueueDepths/the Prometheus label).
type job struct {
	runID     int64
	queueID   int64
	queueName string
	targets   []store.Target
	snapshots map[int64]json.RawMessage
}

// runState tracks a run across its queue jobs.
type runState struct {
	ctx          context.Context
	cancel       context.CancelFunc
	pending      int // queue jobs not yet finished
	total        int // targets in the whole run, for the UI stepper
	done         int // targets that have produced a result
	failed       bool
	canceled     bool
	started      bool // true once any queue has begun executing (running written)
	trigger      string
	scheduleID   *int64
	scheduleName string
}

// Runner owns the queue channels and their workers.
type Runner struct {
	cfg Config

	// enqueueMu serializes the schedule-dedupe check with run creation so
	// concurrent Enqueue calls for the same schedule never create two rows.
	// It guards only that SQLite round-trip, is never held together with
	// r.mu, and no other goroutine needs it.
	enqueueMu sync.Mutex

	mu sync.Mutex
	// queues is keyed by queue id, not name: identity must stay stable
	// across a mid-flight rename so same-queue targets keep serializing
	// through the same channel. queueNames tracks each queue's
	// most-recently-seen display name, updated whenever a job for that id
	// is created, for QueueDepths/the Prometheus label.
	queues     map[int64]chan job
	queueNames map[int64]string
	runs       map[int64]*runState
	closing    bool
	wg         sync.WaitGroup
	started    bool
}

// New returns a Runner. Call Start before Enqueue.
func New(cfg Config) *Runner {
	if cfg.QueueCap <= 0 {
		cfg.QueueCap = 32
	}
	if cfg.Grace <= 0 {
		cfg.Grace = 60 * time.Second
	}
	if cfg.TestTimeout <= 0 {
		cfg.TestTimeout = 10 * time.Minute
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Sink == nil {
		cfg.Sink = Sinks(nil)
	}
	return &Runner{
		cfg:        cfg,
		queues:     map[int64]chan job{},
		queueNames: map[int64]string{},
		runs:       map[int64]*runState{},
	}
}

// Start marks the runner open for work. Queue workers spawn on first use.
func (r *Runner) Start() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.started = true
}

// queueChan returns (creating if needed) the channel for a queue id, plus
// its worker, and records name as that queue's freshest known display
// name. Caller holds r.mu.
func (r *Runner) queueChan(id int64, name string) chan job {
	r.queueNames[id] = name
	if ch, ok := r.queues[id]; ok {
		return ch
	}
	ch := make(chan job, r.cfg.QueueCap)
	r.queues[id] = ch
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		for j := range ch {
			r.execute(j)
		}
	}()
	return ch
}

// filterRunnable drops disabled targets for every trigger except "manual",
// which may deliberately run a disabled target (e.g. a one-off manual test
// while a target is paused).
func filterRunnable(targets []store.Target, trigger string) []store.Target {
	if trigger == "manual" {
		return targets
	}
	out := make([]store.Target, 0, len(targets))
	for _, t := range targets {
		if t.Enabled {
			out = append(out, t)
		}
	}
	return out
}

// Enqueue creates a run row and queues its targets, grouped by queue. It
// returns the run id without waiting for the test to finish. When the
// request names a schedule that already has a queued or running run, the
// existing run id is returned and nothing new is queued.
//
// The schedule dedupe check and run creation happen under enqueueMu (a
// separate lock from r.mu) so SQLite I/O is never done while r.mu is held.
// r.mu is then taken only for the closing check, the run-map insert, the
// "queued" publish and the queue-channel sends, which keeps Enqueue mutually
// exclusive with Shutdown closing those channels (so a send on a closed
// channel can never happen) without blocking other goroutines on I/O. The
// "queued" event is published under r.mu, before any queue send, so clients
// can never observe "running" before "queued" (hub.Publish is in-process
// and non-blocking, so this costs nothing meaningful under the lock).
// Failure-path publishes (queue full, shutting down) happen after r.mu is
// released, since they involve a SetRunStatus store call first.
func (r *Runner) Enqueue(ctx context.Context, req RunRequest) (int64, error) {
	targets, err := r.cfg.Store.ListTargetsByIDs(ctx, req.TargetIDs)
	if err != nil {
		return 0, err
	}
	targets = filterRunnable(targets, req.Trigger)
	if len(targets) == 0 {
		return 0, ErrNoTargets
	}

	// Grouped by QueueID (the stable identity), not QueueName: a queue
	// rename mid-run must not split what should be one serialized group
	// into two, or merge two different queues that briefly share a name.
	byQueue := map[int64][]store.Target{}
	queueNames := map[int64]string{}
	order := []int64{}
	for _, t := range targets {
		if _, seen := byQueue[t.QueueID]; !seen {
			order = append(order, t.QueueID)
		}
		byQueue[t.QueueID] = append(byQueue[t.QueueID], t)
		queueNames[t.QueueID] = t.QueueName
	}

	r.enqueueMu.Lock()
	if req.ScheduleID != nil {
		if id, ok, err := r.cfg.Store.QueuedRunForSchedule(ctx, *req.ScheduleID); err != nil {
			r.enqueueMu.Unlock()
			return 0, err
		} else if ok {
			r.enqueueMu.Unlock()
			return id, nil
		}
	}
	runID, err := r.cfg.Store.CreateRun(ctx, req.Trigger, req.ScheduleID)
	r.enqueueMu.Unlock()
	if err != nil {
		return 0, err
	}

	runCtx, cancel := context.WithCancel(context.Background())

	scheduleName := ""
	if req.ScheduleID != nil {
		if sc, err := r.cfg.Store.GetSchedule(ctx, *req.ScheduleID); err == nil {
			scheduleName = sc.Name
		}
	}

	r.mu.Lock()
	if r.closing || !r.started {
		r.mu.Unlock()
		cancel()
		// The run row was already created before we could see closing; it
		// never gets a queue job, so resolve it as canceled rather than
		// leaving it stuck at "queued".
		err := r.cfg.Store.SetRunStatus(context.Background(), runID, "canceled", "shutting down")
		if err != nil && !errors.Is(err, store.ErrInvalidTransition) {
			r.cfg.Logger.Error("set run status", "error", err, "run_id", runID)
		} else if err == nil {
			r.publishRun(runID, "canceled", "shutting down", len(targets), 0)
		}
		return 0, ErrShuttingDown
	}
	r.runs[runID] = &runState{
		ctx: runCtx, cancel: cancel, pending: len(order), total: len(targets),
		trigger: req.Trigger, scheduleID: req.ScheduleID, scheduleName: scheduleName,
	}

	chans := make([]chan job, 0, len(order))
	for _, qid := range order {
		chans = append(chans, r.queueChan(qid, queueNames[qid]))
	}

	// Publish "queued" before any queue send: a worker cannot even attempt to
	// dequeue this run's job until it exists in a queue channel, so
	// publishing here, still under r.mu and before those sends, guarantees
	// clients never observe "running" before "queued". Hub.Publish is
	// in-process and non-blocking (buffered channels with a default case),
	// so this adds no meaningful time under the lock.
	r.publishRun(runID, "queued", "", len(targets), 0)

	queueFull := false
	for i, qid := range order {
		select {
		case chans[i] <- job{runID: runID, queueID: qid, queueName: queueNames[qid], targets: byQueue[qid], snapshots: req.Snapshots}:
		default:
			queueFull = true
		}
		if queueFull {
			break
		}
	}
	if queueFull {
		cancel()
		delete(r.runs, runID)
	}
	r.mu.Unlock()

	if queueFull {
		_ = r.cfg.Store.SetRunStatus(ctx, runID, "failed", "queue full")
		r.publishRun(runID, "failed", "queue full", len(targets), 0)
		return 0, ErrQueueFull
	}
	return runID, nil
}

// Cancel aborts an in-flight or queued run. It reports whether the run was
// known to the runner. Setting canceled and calling the run's CancelFunc
// happen atomically under r.mu, together with execute's own canceled/started
// check, so a queue can never observe canceled==false and go on to write
// "running" after Cancel has already committed to canceling the run. When
// the run has not started any queue yet, Cancel claims it by deleting it
// from r.runs (the same claim discipline claimForForceCancel and finishQueue
// use) and persists the canceled status itself; a queue job for this run
// still sitting in a channel will later find the run gone from r.runs and
// return without writing or publishing anything, so the terminal status is
// never written twice.
func (r *Runner) Cancel(runID int64) bool {
	r.mu.Lock()
	st, ok := r.runs[runID]
	if !ok {
		r.mu.Unlock()
		return false
	}
	st.canceled = true
	notStarted := !st.started
	total := st.total
	if notStarted {
		delete(r.runs, runID)
	}
	st.cancel()
	r.mu.Unlock()

	if notStarted {
		err := r.cfg.Store.SetRunStatus(context.Background(), runID, "canceled", "")
		if err != nil && !errors.Is(err, store.ErrInvalidTransition) {
			r.cfg.Logger.Error("cancel: set run status", "error", err, "run_id", runID)
		} else if err == nil {
			r.publishRun(runID, "canceled", "", total, 0)
		}
	}
	return true
}

// execute runs one queue's targets sequentially.
func (r *Runner) execute(j job) {
	r.mu.Lock()
	st, ok := r.runs[j.runID]
	if !ok {
		r.mu.Unlock()
		return
	}
	ctx := st.ctx
	if st.canceled || r.closing || ctx.Err() != nil {
		// Canceled (by Cancel, Shutdown, or the run's context otherwise
		// done) while still queued: never write "running" or start a
		// target, so started_at stays unset. Checking st.canceled and
		// r.closing here, under the same lock Cancel and Shutdown use to
		// set them, closes the race where a queue reads ctx.Err()==nil right
		// before one of them commits and goes on to write "running" anyway.
		r.mu.Unlock()
		r.finishQueue(j.runID, false, true)
		return
	}
	st.started = true
	total := st.total
	r.mu.Unlock()

	if err := r.cfg.Store.SetRunStatus(context.Background(), j.runID, "running", ""); err == nil {
		r.publishRun(j.runID, "running", "", total, r.doneCount(j.runID))
	}

	queueFailed := false
	for _, t := range j.targets {
		if ctx.Err() != nil {
			break
		}
		if failed := r.runTarget(ctx, j.runID, j.queueName, t, j.snapshots[t.ID]); failed {
			queueFailed = true
		}
		// The result row for this target has landed: advance the stepper.
		if total, done, ok := r.markTargetDone(j.runID); ok {
			r.publishRun(j.runID, "running", "", total, done)
		}
	}
	r.finishQueue(j.runID, queueFailed, ctx.Err() != nil)
}

// runTarget executes one target and writes exactly one result row. It
// reports whether the result failed. When override is non-empty (a
// re-execute replaying a stored result's options_snapshot), it is used as
// the run's options instead of the target's current live options.
func (r *Runner) runTarget(ctx context.Context, runID int64, queue string, t store.Target, override json.RawMessage) bool {
	started := r.cfg.Now().UTC()
	options := t.Options
	if len(override) > 0 {
		options = override
	}
	snapshot := options
	if len(snapshot) == 0 {
		snapshot = json.RawMessage(`{}`)
	}
	tid := t.ID
	res := &store.Result{
		RunID: &runID, TargetID: &tid, TargetName: t.Name, Engine: t.Engine,
		OptionsSnapshot: snapshot,
		StartedAt:       started.Format("2006-01-02T15:04:05.000Z"),
	}

	r.mu.Lock()
	meta := ResultMeta{RunID: runID, QueueName: queue}
	if st, ok := r.runs[runID]; ok {
		meta.Trigger = st.trigger
		meta.ScheduleID = st.scheduleID
		meta.ScheduleName = st.scheduleName
	}
	r.mu.Unlock()

	eng, ok := r.cfg.Registry.Get(t.Engine)
	if !ok {
		res.Status, res.Error = "failed", fmt.Sprintf("unknown engine %q", t.Engine)
		r.storeResult(res, meta)
		return true
	}

	testCtx, cancel := context.WithTimeout(ctx, r.cfg.TestTimeout)
	defer cancel()

	var (
		mu        sync.Mutex
		lastSent  time.Time
		lastPhase engine.Phase
	)
	prog := func(p engine.Progress) {
		mu.Lock()
		now := r.cfg.Now()
		terminal := p.Phase == engine.PhaseDone || p.Phase == engine.PhaseError
		if !terminal && p.Phase == lastPhase && now.Sub(lastSent) < progressInterval {
			mu.Unlock()
			return
		}
		lastSent, lastPhase = now, p.Phase
		mu.Unlock()

		r.cfg.Hub.Publish(r.cfg.Hub.Marshal(sse.EventProgress, ProgressEvent{
			RunID: runID, TargetID: t.ID, Engine: t.Engine, Progress: p,
		}))
	}

	out, err := eng.Run(testCtx, options, prog)
	res.DurationMs = r.cfg.Now().UTC().Sub(started).Milliseconds()
	if err != nil {
		res.Status, res.Error = "failed", err.Error()
		r.storeResult(res, meta)
		return true
	}
	res.Status = "ok"
	res.DownloadBps, res.UploadBps = out.DownloadBps, out.UploadBps
	res.PingMs, res.JitterMs, res.PacketLossPct = out.PingMs, out.JitterMs, out.PacketLossPct
	res.BytesDown, res.BytesUp = out.BytesDown, out.BytesUp
	res.ServerID, res.ServerName, res.ServerHost = out.ServerID, out.ServerName, out.ServerHost
	res.ISP, res.ExternalIP, res.ResultURL = out.ISP, out.ExternalIP, out.ResultURL
	res.Raw = out.Raw
	// A failure to persist the result means the run must not end up "done"
	// with no result row for this target, even though the test itself
	// succeeded.
	return !r.storeResult(res, meta)
}

// storeResult writes the row, publishes the result event, and notifies the
// configured sink. It reports whether the row was written. The sink is
// called after InsertResult (so res.ID is set) and after the hub publish,
// with context.Background() since the run's own ctx may already be
// canceled by the time the result lands.
func (r *Runner) storeResult(res *store.Result, meta ResultMeta) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := r.cfg.Store.InsertResult(ctx, res); err != nil {
		r.cfg.Logger.Error("store result", "error", err, "target", res.TargetName, "run_id", res.RunID)
		return false
	}
	r.cfg.Hub.Publish(r.cfg.Hub.Marshal(sse.EventResult, res))
	r.cfg.Sink.OnResult(context.Background(), res, meta)
	return true
}

// finishQueue records a queue job's outcome and, when it is the run's last
// queue, writes the terminal run status.
func (r *Runner) finishQueue(runID int64, failed, canceled bool) {
	r.mu.Lock()
	st, ok := r.runs[runID]
	if !ok {
		r.mu.Unlock()
		return
	}
	if failed {
		st.failed = true
	}
	if canceled {
		st.canceled = true
	}
	st.pending--
	if st.pending > 0 {
		r.mu.Unlock()
		return
	}
	delete(r.runs, runID)
	total, done := st.total, st.done
	status := "done"
	switch {
	case st.canceled:
		status = "canceled"
	case st.failed:
		status = "failed"
	}
	st.cancel()
	r.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := r.cfg.Store.SetRunStatus(ctx, runID, status, "")
	if err != nil && !errors.Is(err, store.ErrInvalidTransition) {
		r.cfg.Logger.Error("set run status", "error", err, "run_id", runID)
	}
	if err == nil {
		r.publishRun(runID, status, "", total, done)
	}
}

// publishRun emits the run SSE event. total/done drive the UI's per-target
// stepper; they are passed explicitly because some callers publish after
// the runState has already been removed from r.runs.
func (r *Runner) publishRun(runID int64, status, errMsg string, total, done int) {
	r.cfg.Hub.Publish(r.cfg.Hub.Marshal(sse.EventRun, map[string]any{
		"run_id": runID, "status": status, "error": errMsg,
		"targets_total": total, "targets_done": done,
	}))
}

// markTargetDone increments the run's completed-target count and returns
// the new totals, or ok=false when the run is no longer tracked.
func (r *Runner) markTargetDone(runID int64) (total, done int, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	st, ok := r.runs[runID]
	if !ok {
		return 0, 0, false
	}
	st.done++
	return st.total, st.done, true
}

// doneCount reports the run's current stepper counts, 0/0 when untracked.
func (r *Runner) doneCount(runID int64) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	if st, ok := r.runs[runID]; ok {
		return st.done
	}
	return 0
}

// claimForForceCancel atomically removes id from r.runs if it is still
// present there, reporting whether it did, along with its stepper counts.
// Deleting and checking under the same r.mu critical section is what makes
// this race-free against finishQueue, which deletes the same map entry
// (under r.mu too) right before it persists the run's real terminal status:
// at most one of the two calls can observe the entry and delete it, so
// exactly one of them gets to write the run's final status — the loser must
// not overwrite it.
func (r *Runner) claimForForceCancel(id int64) (total, done int, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	st, ok := r.runs[id]
	if !ok {
		return 0, 0, false
	}
	delete(r.runs, id)
	return st.total, st.done, true
}

// QueueDepths reports each queue's current queue depth (jobs buffered, not
// yet dequeued by the queue worker), for metrics, keyed by each queue's
// most-recently-seen display name (channels themselves are keyed by id,
// which is not meaningful to show on a Prometheus label).
func (r *Runner) QueueDepths() map[string]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	depths := make(map[string]int, len(r.queues))
	for id, ch := range r.queues {
		depths[r.queueNames[id]] = len(ch)
	}
	return depths
}

// Shutdown stops accepting work, waits up to Grace for in-flight queues,
// then force-cancels what is left and marks only the runs still genuinely
// in flight as canceled — a run finishQueue already resolved to done/failed
// is never relabeled. If ctx is done before everything settles, Shutdown
// returns ctx.Err().
func (r *Runner) Shutdown(ctx context.Context) error {
	r.mu.Lock()
	if r.closing {
		r.mu.Unlock()
		return nil
	}
	r.closing = true
	for _, ch := range r.queues {
		close(ch)
	}
	r.mu.Unlock()

	done := make(chan struct{})
	go func() { r.wg.Wait(); close(done) }()

	graceTimer := time.NewTimer(r.cfg.Grace)
	defer graceTimer.Stop()
	gaveUp := false
	select {
	case <-done:
		return nil
	case <-graceTimer.C:
	case <-ctx.Done():
		gaveUp = true
	}

	r.mu.Lock()
	stuck := make([]int64, 0, len(r.runs))
	for id, st := range r.runs {
		st.canceled = true
		st.cancel()
		stuck = append(stuck, id)
	}
	r.mu.Unlock()

	if !gaveUp {
		select {
		case <-done:
		case <-ctx.Done():
			gaveUp = true
		}
	}

	for _, id := range stuck {
		total, done, ok := r.claimForForceCancel(id)
		if !ok {
			// finishQueue already claimed and persisted the run's real
			// terminal status (done/failed/canceled); do not overwrite it.
			continue
		}
		markCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := r.cfg.Store.SetRunStatus(markCtx, id, "canceled", "server shutdown")
		if err != nil && !errors.Is(err, store.ErrInvalidTransition) {
			r.cfg.Logger.Error("mark canceled", "error", err, "run_id", id)
		} else if err == nil {
			r.publishRun(id, "canceled", "server shutdown", total, done)
		}
		cancel()
	}

	if gaveUp {
		return ctx.Err()
	}
	return nil
}
