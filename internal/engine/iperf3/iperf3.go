package iperf3

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/metril/speedtest-tracker/internal/engine"
	"github.com/metril/speedtest-tracker/internal/engine/execx"
)

// stderrTailLimit bounds how much of the CLI's stderr we keep around to
// fold into an error message; stderr is not expected to be large.
const stderrTailLimit = 4 << 10 // 4 KiB

// Engine runs the iperf3 client at Bin.
type Engine struct {
	Bin string
	// ForceSummary skips the --json-stream capability probe and always uses
	// the single -J summary document. Used by tests.
	ForceSummary bool

	probeOnce   sync.Once
	probeStream bool
}

// supportsStream probes --json-stream capability once per Engine and caches
// the result. A failed or unparseable probe falls back to summary mode (-J)
// rather than aborting the run: some iperf3 builds don't accept --version
// the way we expect, but can still run a test.
func (e *Engine) supportsStream(ctx context.Context) bool {
	e.probeOnce.Do(func() {
		ok, err := supportsJSONStream(ctx, e.Bin)
		if err != nil {
			e.probeStream = false
			return
		}
		e.probeStream = ok
	})
	return e.probeStream
}

// New returns an iperf3 engine using the binary at bin.
func New(bin string) *Engine { return &Engine{Bin: bin} }

// Name implements engine.Engine.
func (e *Engine) Name() string { return "iperf3" }

// Validate implements engine.Engine.
func (e *Engine) Validate(opts json.RawMessage) error {
	_, err := parseOptions(opts)
	return err
}

// Run implements engine.Engine. With iperf3 >= 3.17 it streams per-interval
// progress via --json-stream; otherwise it parses the -J summary at the end.
func (e *Engine) Run(ctx context.Context, opts json.RawMessage, prog func(engine.Progress)) (*engine.Result, error) {
	o, err := parseOptions(opts)
	if err != nil {
		return nil, err
	}
	stream := false
	if !e.ForceSummary {
		stream = e.supportsStream(ctx)
	}

	cmd := execx.Command(ctx, e.Bin, buildArgs(o, stream)...)
	if o.Password != "" {
		cmd.Env = append(os.Environ(), "IPERF3_PASSWORD="+o.Password)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr := execx.NewTailBuffer(stderrTailLimit)
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", e.Bin, err)
	}

	var (
		res      *engine.Result
		parseErr error
	)
	if stream {
		res, parseErr = parseStreamJSONL(stdout, o, prog)
	} else {
		engine.Emit(prog, engine.Progress{Phase: engine.PhaseConnecting, ServerName: o.Host})
		var buf bytes.Buffer
		if _, err := io.Copy(&buf, stdout); err != nil {
			parseErr = err
		} else {
			res, parseErr = parseSummary(buf.Bytes(), o)
			if parseErr == nil {
				engine.Emit(prog, engine.Progress{
					Phase: engine.PhaseDone, Progress: 1, Bps: maxBps(res),
					JitterMs: res.JitterMs, LossPct: res.PacketLossPct,
					ElapsedMs: int64(o.DurationS) * 1000, ServerName: o.Host,
				})
			}
		}
	}
	_, _ = io.Copy(io.Discard, stdout) // drain so the child never blocks
	waitErr := cmd.Wait()

	if ctx.Err() != nil && res == nil {
		return nil, ctx.Err()
	}
	if res != nil && parseErr == nil {
		// A valid summary outranks a nonzero exit: some iperf3 versions exit
		// nonzero after already printing a valid result document.
		return res, nil
	}
	if parseErr != nil && !errors.Is(parseErr, errNoDocument) {
		return nil, parseErr // iperf3's own JSON error message beats the exit code
	}
	if waitErr != nil {
		if tail := stderr.String(); tail != "" {
			return nil, fmt.Errorf("%s: %w: %s", e.Bin, waitErr, tail)
		}
		return nil, fmt.Errorf("%s: %w", e.Bin, waitErr)
	}
	return res, parseErr
}
