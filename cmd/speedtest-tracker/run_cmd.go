package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/metril/speedtest-tracker/internal/config"
	"github.com/metril/speedtest-tracker/internal/engine"
	"github.com/metril/speedtest-tracker/internal/engine/cloudflare"
	"github.com/metril/speedtest-tracker/internal/engine/fake"
	"github.com/metril/speedtest-tracker/internal/engine/iperf3"
	"github.com/metril/speedtest-tracker/internal/engine/ookla"
	"github.com/metril/speedtest-tracker/internal/settings"
	"github.com/metril/speedtest-tracker/internal/store"
)

// buildRegistry wires every engine using the Engines settings section.
func buildRegistry(e settings.Engines) *engine.Registry {
	r := engine.NewRegistry()
	r.Register(fake.New())
	r.Register(ookla.New(e.SpeedtestBin, ookla.Config{
		AcceptLicense: e.OoklaAcceptLicense,
		AcceptGDPR:    e.OoklaAcceptGDPR,
	}))
	r.Register(cloudflare.New(nil))
	r.Register(iperf3.New(e.Iperf3Bin))
	return r
}

// runCmd implements `speedtest-tracker run --engine <name> --opts '<json>'`.
// It reads the same settings database as the server, at ST_DB_PATH,
// creating it (with the seeded defaults) if it does not exist yet, so
// binary paths and the Ookla consent flags match whatever is configured
// there. --speedtest-bin/--iperf3-bin, if given, override just the binary
// path for this invocation. The Result JSON goes to stdout; progress events
// go to stderr, one JSON object per line. It returns the process exit code.
func runCmd(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		name         = fs.String("engine", "", "engine name: ookla, cloudflare, iperf3, fake")
		opts         = fs.String("opts", "{}", "engine options as a JSON object")
		timeout      = fs.Duration("timeout", 5*time.Minute, "overall timeout")
		speedtestBin = fs.String("speedtest-bin", "", "override the Ookla speedtest binary path (default: from settings)")
		iperf3Bin    = fs.String("iperf3-bin", "", "override the iperf3 binary path (default: from settings)")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg := config.Load()
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	defer db.Close()

	st, err := settings.New(ctx, db)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	eng, err := st.Engines(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if *speedtestBin != "" {
		eng.SpeedtestBin = *speedtestBin
	}
	if *iperf3Bin != "" {
		eng.Iperf3Bin = *iperf3Bin
	}

	reg := buildRegistry(eng)
	e, ok := reg.Get(*name)
	if !ok {
		fmt.Fprintf(stderr, "unknown engine %q (have %v)\n", *name, reg.Names())
		return 2
	}
	raw := json.RawMessage(*opts)
	if err := e.Validate(raw); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()

	enc := json.NewEncoder(stderr)
	res, err := e.Run(ctx, raw, func(p engine.Progress) { _ = enc.Encode(p) })
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	out := json.NewEncoder(stdout)
	out.SetIndent("", "  ")
	if err := out.Encode(res); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
