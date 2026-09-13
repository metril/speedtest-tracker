// Package fake provides a deterministic Engine used by tests and by
// end-to-end runner/scheduler exercises. It performs no I/O.
package fake

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/metril/speedtest-tracker/internal/engine"
)

// Options are the fake engine's options.
type Options struct {
	Fail        bool    `json:"fail"`
	DownloadBps float64 `json:"download_bps"`
	UploadBps   float64 `json:"upload_bps"`
}

// Engine is a deterministic engine. Steps is the number of progress events
// per transfer phase; Delay is slept between events (zero in tests).
type Engine struct {
	Steps int
	Delay time.Duration
}

// New returns a fake engine with five steps per phase and no delay.
func New() *Engine { return &Engine{Steps: 5} }

// Name implements engine.Engine.
func (e *Engine) Name() string { return "fake" }

// Validate implements engine.Engine.
func (e *Engine) Validate(opts json.RawMessage) error {
	_, err := parse(opts)
	return err
}

func parse(opts json.RawMessage) (Options, error) {
	o := Options{DownloadBps: 100_000_000, UploadBps: 50_000_000}
	if len(opts) == 0 {
		return o, nil
	}
	if err := json.Unmarshal(opts, &o); err != nil {
		return Options{}, err
	}
	if o.DownloadBps == 0 {
		o.DownloadBps = 100_000_000
	}
	if o.UploadBps == 0 {
		o.UploadBps = 50_000_000
	}
	return o, nil
}

// Run implements engine.Engine with a fixed, reproducible event stream.
func (e *Engine) Run(ctx context.Context, opts json.RawMessage, prog func(engine.Progress)) (*engine.Result, error) {
	o, err := parse(opts)
	if err != nil {
		return nil, err
	}
	steps := e.Steps
	if steps <= 0 {
		steps = 5
	}
	var elapsed int64

	emit := func(p engine.Progress) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		p.ServerName = "fake-server"
		p.ElapsedMs = elapsed
		engine.Emit(prog, p)
		elapsed += 200
		if e.Delay > 0 {
			time.Sleep(e.Delay)
		}
		return nil
	}

	if err := emit(engine.Progress{Phase: engine.PhaseConnecting}); err != nil {
		return nil, err
	}
	if o.Fail {
		_ = emit(engine.Progress{Phase: engine.PhaseError})
		return nil, errors.New("fake: forced failure")
	}
	for i := 1; i <= steps; i++ {
		frac := float64(i) / float64(steps)
		if err := emit(engine.Progress{Phase: engine.PhasePing, Progress: frac, PingMs: 12.5, JitterMs: 1.5}); err != nil {
			return nil, err
		}
	}
	for _, ph := range []struct {
		phase engine.Phase
		bps   float64
	}{{engine.PhaseDownload, o.DownloadBps}, {engine.PhaseUpload, o.UploadBps}} {
		for i := 1; i <= steps; i++ {
			frac := float64(i) / float64(steps)
			if err := emit(engine.Progress{Phase: ph.phase, Progress: frac, Bps: ph.bps * frac}); err != nil {
				return nil, err
			}
		}
	}
	if err := emit(engine.Progress{Phase: engine.PhaseDone, Progress: 1, Bps: o.DownloadBps}); err != nil {
		return nil, err
	}

	return &engine.Result{
		DownloadBps:   o.DownloadBps,
		UploadBps:     o.UploadBps,
		PingMs:        12.5,
		JitterMs:      1.5,
		PacketLossPct: 0,
		BytesDown:     125_000_000,
		BytesUp:       62_500_000,
		ServerID:      "fake-1",
		ServerName:    "fake-server",
		ServerHost:    "fake.invalid:8080",
		ISP:           "Fake ISP",
		ExternalIP:    "203.0.113.1",
		Raw:           json.RawMessage(`{"engine":"fake"}`),
	}, nil
}
