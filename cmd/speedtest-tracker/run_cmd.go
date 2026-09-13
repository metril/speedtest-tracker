package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/metril/speedtest-tracker/internal/engine"
	"github.com/metril/speedtest-tracker/internal/engine/cloudflare"
	"github.com/metril/speedtest-tracker/internal/engine/fake"
	"github.com/metril/speedtest-tracker/internal/engine/iperf3"
	"github.com/metril/speedtest-tracker/internal/engine/ookla"
	"github.com/metril/speedtest-tracker/internal/settings"
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
// The Result JSON goes to stdout; progress events go to stderr, one JSON
// object per line. It returns the process exit code.
func runCmd(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		name         = fs.String("engine", "", "engine name: ookla, cloudflare, iperf3, fake")
		opts         = fs.String("opts", "{}", "engine options as a JSON object")
		timeout      = fs.Duration("timeout", 5*time.Minute, "overall timeout")
		speedtestBin = fs.String("speedtest-bin", "speedtest", "path to the Ookla speedtest binary")
		iperf3Bin    = fs.String("iperf3-bin", "iperf3", "path to the iperf3 binary")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	reg := buildRegistry(settings.Engines{
		SpeedtestBin:       *speedtestBin,
		Iperf3Bin:          *iperf3Bin,
		OoklaAcceptLicense: true,
		OoklaAcceptGDPR:    true,
	})
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
