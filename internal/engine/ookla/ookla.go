package ookla

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"

	"github.com/metril/speedtest-tracker/internal/engine"
)

// Options are the Ookla engine's per-target options.
type Options struct {
	ServerID int64 `json:"server_id,omitempty"`
}

// Config holds the settings-derived consent flags.
type Config struct {
	AcceptLicense bool
	AcceptGDPR    bool
}

// Engine runs the Ookla speedtest CLI at Bin.
type Engine struct {
	Bin string
	Cfg Config
}

// New returns an Ookla engine using the binary at bin.
func New(bin string, cfg Config) *Engine { return &Engine{Bin: bin, Cfg: cfg} }

// Name implements engine.Engine.
func (e *Engine) Name() string { return "ookla" }

// Validate implements engine.Engine.
func (e *Engine) Validate(opts json.RawMessage) error {
	_, err := parseOptions(opts)
	return err
}

func parseOptions(opts json.RawMessage) (Options, error) {
	var o Options
	if len(opts) == 0 {
		return o, nil
	}
	if err := json.Unmarshal(opts, &o); err != nil {
		return Options{}, fmt.Errorf("ookla options: %w", err)
	}
	if o.ServerID < 0 {
		return Options{}, errors.New("ookla options: server_id must be positive")
	}
	return o, nil
}

// Args returns the CLI arguments for one test run.
func (e *Engine) Args(o Options) []string {
	args := []string{"-f", "jsonl", "--progress=yes"}
	if e.Cfg.AcceptLicense {
		args = append(args, "--accept-license")
	}
	if e.Cfg.AcceptGDPR {
		args = append(args, "--accept-gdpr")
	}
	if o.ServerID > 0 {
		args = append(args, "-s", strconv.FormatInt(o.ServerID, 10))
	}
	return args
}

// Run implements engine.Engine: it streams the CLI's JSONL output, emitting
// Progress as it arrives, and returns the final Result.
func (e *Engine) Run(ctx context.Context, opts json.RawMessage, prog func(engine.Progress)) (*engine.Result, error) {
	o, err := parseOptions(opts)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, e.Bin, e.Args(o)...)
	cmd.SysProcAttr = sysProcAttr()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", e.Bin, err)
	}

	res, parseErr := parseStream(stdout, prog)
	_, _ = io.Copy(io.Discard, stdout) // drain so the child never blocks
	waitErr := cmd.Wait()

	if parseErr != nil {
		return nil, parseErr // the CLI's own error message is the useful one
	}
	if waitErr != nil {
		return nil, fmt.Errorf("%s: %w", e.Bin, waitErr)
	}
	return res, nil
}
