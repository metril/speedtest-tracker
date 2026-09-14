// Package runner executes speed tests. It keeps one buffered queue and one
// worker goroutine per lane so a LAN iperf3 test and a WAN Ookla test can
// run at the same time while two WAN tests never overlap.
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
	ErrQueueFull    = errors.New("runner: lane queue full")
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
	Logger      *slog.Logger
	QueueCap    int           // per-lane channel capacity, default 32
	Grace       time.Duration // shutdown grace period, default 60s
	TestTimeout time.Duration // per-target timeout, default 10m
	Now         func() time.Time
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

// job is one lane's share of a run.
type job struct {
	runID     int64
	targets   []store.Target
	snapshots map[int64]json.RawMessage
}

// runState tracks a run across its lane jobs.
type runState struct {
	ctx      context.Context
	cancel   context.CancelFunc
	pending  int // lane jobs not yet finished
	failed   bool
	canceled bool
	started  bool // true once any lane has begun executing (running written)
}

// Runner owns the lane queues and their workers.
type Runner struct {
	cfg Config

	mu      sync.Mutex
	lanes   map[string]chan job
	runs    map[int64]*runState
	closing bool
	wg      sync.WaitGroup
	started bool
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
	return &Runner{
		cfg:   cfg,
		lanes: map[string]chan job{},
		runs:  map[int64]*runState{},
	}
}

// Start marks the runner open for work. Lane workers spawn on first use.
func (r *Runner) Start() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.started = true
}

// laneChan returns (creating if needed) the queue for a lane, plus its
// worker. Caller holds r.mu.
func (r *Runner) laneChan(lane string) chan job {
	if ch, ok := r.lanes[lane]; ok {
		return ch
	}
	ch := make(chan job, r.cfg.QueueCap)
	r.lanes[lane] = ch
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		for j := range ch {
			r.execute(j)
		}
	}()
	return ch
}

// Enqueue creates a run row and queues its targets, grouped by lane. It
// returns the run id without waiting for the test to finish. When the
// request names a schedule that already has a queued or running run, the
// existing run id is returned and nothing new is queued.
//
// The schedule dedupe check, run creation, and the lane-channel sends all
// happen under r.mu: this makes concurrent Enqueue calls for the same
// schedule race-free (exactly one run gets created) and makes Enqueue
// mutually exclusive with Shutdown closing the lane channels, so a send on
// a closed channel can never happen.
func (r *Runner) Enqueue(ctx context.Context, req RunRequest) (int64, error) {
	targets, err := r.cfg.Store.ListTargetsByIDs(ctx, req.TargetIDs)
	if err != nil {
		return 0, err
	}
	if len(targets) == 0 {
		return 0, ErrNoTargets
	}

	byLane := map[string][]store.Target{}
	order := []string{}
	for _, t := range targets {
		lane := t.Lane
		if lane == "" {
			lane = "wan"
		}
		if _, seen := byLane[lane]; !seen {
			order = append(order, lane)
		}
		byLane[lane] = append(byLane[lane], t)
	}

	r.mu.Lock()
	if r.closing || !r.started {
		r.mu.Unlock()
		return 0, ErrShuttingDown
	}

	if req.ScheduleID != nil {
		if id, ok, err := r.cfg.Store.QueuedRunForSchedule(ctx, *req.ScheduleID); err != nil {
			r.mu.Unlock()
			return 0, err
		} else if ok {
			r.mu.Unlock()
			return id, nil
		}
	}

	runID, err := r.cfg.Store.CreateRun(ctx, req.Trigger, req.ScheduleID)
	if err != nil {
		r.mu.Unlock()
		return 0, err
	}
	runCtx, cancel := context.WithCancel(context.Background())
	r.runs[runID] = &runState{ctx: runCtx, cancel: cancel, pending: len(order)}

	chans := make([]chan job, 0, len(order))
	for _, lane := range order {
		chans = append(chans, r.laneChan(lane))
	}

	// Publish "queued" before any lane can possibly move the run to
	// "running", so clients never observe running before queued.
	r.publishRun(runID, "queued", "")

	queueFull := false
	for i, lane := range order {
		select {
		case chans[i] <- job{runID: runID, targets: byLane[lane], snapshots: req.Snapshots}:
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
		_ = r.cfg.Store.SetRunStatus(ctx, runID, "failed", "lane queue full")
		r.publishRun(runID, "failed", "lane queue full")
		return 0, ErrQueueFull
	}
	return runID, nil
}

// Cancel aborts an in-flight or queued run. It reports whether the run was
// known to the runner. Setting canceled and calling the run's CancelFunc
// happen atomically under r.mu, together with execute's own canceled/started
// check, so a lane can never observe canceled==false and go on to write
// "running" after Cancel has already committed to canceling the run. When
// the run has not started any lane yet, the canceled status is persisted
// immediately (rather than waiting for a worker to eventually dequeue it),
// and started_at is never set.
func (r *Runner) Cancel(runID int64) bool {
	r.mu.Lock()
	st, ok := r.runs[runID]
	if !ok {
		r.mu.Unlock()
		return false
	}
	st.canceled = true
	notStarted := !st.started
	st.cancel()
	r.mu.Unlock()

	if notStarted {
		_ = r.cfg.Store.SetRunStatus(context.Background(), runID, "canceled", "")
		r.publishRun(runID, "canceled", "")
	}
	return true
}

// execute runs one lane's targets sequentially.
func (r *Runner) execute(j job) {
	r.mu.Lock()
	st, ok := r.runs[j.runID]
	if !ok {
		r.mu.Unlock()
		return
	}
	ctx := st.ctx
	if st.canceled || ctx.Err() != nil {
		// Canceled (by Cancel, or the run's context otherwise done) while
		// still queued: never write "running" or start a target, so
		// started_at stays unset. Checking st.canceled here, under the same
		// lock Cancel uses to set it, closes the race where a lane reads
		// ctx.Err()==nil right before Cancel commits and goes on to write
		// "running" anyway.
		r.mu.Unlock()
		r.finishLane(j.runID, false, true)
		return
	}
	st.started = true
	r.mu.Unlock()

	if err := r.cfg.Store.SetRunStatus(context.Background(), j.runID, "running", ""); err == nil {
		r.publishRun(j.runID, "running", "")
	}

	laneFailed := false
	for _, t := range j.targets {
		if ctx.Err() != nil {
			break
		}
		if failed := r.runTarget(ctx, j.runID, t, j.snapshots[t.ID]); failed {
			laneFailed = true
		}
	}
	r.finishLane(j.runID, laneFailed, ctx.Err() != nil)
}

// runTarget executes one target and writes exactly one result row. It
// reports whether the result failed. When override is non-empty (a
// re-execute replaying a stored result's options_snapshot), it is used as
// the run's options instead of the target's current live options.
func (r *Runner) runTarget(ctx context.Context, runID int64, t store.Target, override json.RawMessage) bool {
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

	eng, ok := r.cfg.Registry.Get(t.Engine)
	if !ok {
		res.Status, res.Error = "failed", fmt.Sprintf("unknown engine %q", t.Engine)
		r.storeResult(res)
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
		r.storeResult(res)
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
	return !r.storeResult(res)
}

// storeResult writes the row and publishes the result event. It reports
// whether the row was written.
func (r *Runner) storeResult(res *store.Result) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := r.cfg.Store.InsertResult(ctx, res); err != nil {
		r.cfg.Logger.Error("store result", "error", err, "target", res.TargetName, "run_id", res.RunID)
		return false
	}
	r.cfg.Hub.Publish(r.cfg.Hub.Marshal(sse.EventResult, res))
	return true
}

// finishLane records a lane job's outcome and, when it is the run's last
// lane, writes the terminal run status.
func (r *Runner) finishLane(runID int64, failed, canceled bool) {
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
	if err := r.cfg.Store.SetRunStatus(ctx, runID, status, ""); err != nil {
		r.cfg.Logger.Error("set run status", "error", err, "run_id", runID)
	}
	r.publishRun(runID, status, "")
}

// publishRun emits the run SSE event.
func (r *Runner) publishRun(runID int64, status, errMsg string) {
	r.cfg.Hub.Publish(r.cfg.Hub.Marshal(sse.EventRun, map[string]any{
		"run_id": runID, "status": status, "error": errMsg,
	}))
}

// claimForForceCancel atomically removes id from r.runs if it is still
// present there, reporting whether it did. Deleting and checking under the
// same r.mu critical section is what makes this race-free against
// finishLane, which deletes the same map entry (under r.mu too) right
// before it persists the run's real terminal status: at most one of the
// two calls can observe the entry and delete it, so exactly one of them
// gets to write the run's final status — the loser must not overwrite it.
func (r *Runner) claimForForceCancel(id int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.runs[id]; !ok {
		return false
	}
	delete(r.runs, id)
	return true
}

// Shutdown stops accepting work, waits up to Grace for in-flight lanes,
// then force-cancels what is left and marks only the runs still genuinely
// in flight as canceled — a run finishLane already resolved to done/failed
// is never relabeled. If ctx is done before everything settles, Shutdown
// returns ctx.Err().
func (r *Runner) Shutdown(ctx context.Context) error {
	r.mu.Lock()
	if r.closing {
		r.mu.Unlock()
		return nil
	}
	r.closing = true
	for _, ch := range r.lanes {
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
		if !r.claimForForceCancel(id) {
			// finishLane already claimed and persisted the run's real
			// terminal status (done/failed/canceled); do not overwrite it.
			continue
		}
		markCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := r.cfg.Store.SetRunStatus(markCtx, id, "canceled", "server shutdown"); err != nil {
			r.cfg.Logger.Error("mark canceled", "error", err, "run_id", id)
		} else {
			r.publishRun(id, "canceled", "server shutdown")
		}
		cancel()
	}

	if gaveUp {
		return ctx.Err()
	}
	return nil
}
