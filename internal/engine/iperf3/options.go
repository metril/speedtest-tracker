// Package iperf3 runs the iperf3 client and parses its JSON output.
package iperf3

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

// Options are the iperf3 engine's per-target options.
type Options struct {
	Host string `json:"host"`
	// Hosts is an ordered list of hosts the runner rotates through, one
	// per run (see internal/runner/rotate.go), sharing Port and every
	// other option below. Once rotation has picked one, it rewrites the
	// options to carry Host alone before Run ever sees them; parseOptions
	// still folds a single-entry list into Host itself, so a list that
	// never grew past one entry (or reached Run outside the runner)
	// behaves the same as Host.
	Hosts []string `json:"hosts,omitempty"`
	Port  int      `json:"port,omitempty"`
	// PortRangeEnd, when greater than Port, makes Run retry on
	// consecutive ports up to this one (still capped at 5 attempts total)
	// whenever the server reports it is busy running another test. Zero
	// (or <= Port) means no retry: a busy server fails the run outright.
	PortRangeEnd     int    `json:"port_range_end,omitempty"`
	Protocol         string `json:"protocol,omitempty"` // tcp or udp
	Reverse          bool   `json:"reverse,omitempty"`
	Bidir            bool   `json:"bidir,omitempty"`
	Parallel         int    `json:"parallel,omitempty"`
	DurationS        int    `json:"duration_s,omitempty"`
	UDPBitrate       string `json:"udp_bitrate,omitempty"`
	Bind             string `json:"bind,omitempty"`
	Username         string `json:"username,omitempty"`
	Password         string `json:"password,omitempty"`
	RSAPublicKeyPath string `json:"rsa_public_key_path,omitempty"`
}

func parseOptions(opts json.RawMessage) (Options, error) {
	var o Options
	if len(opts) > 0 {
		if err := json.Unmarshal(opts, &o); err != nil {
			return Options{}, fmt.Errorf("iperf3 options: %w", err)
		}
	}
	for _, h := range o.Hosts {
		if h == "" {
			return Options{}, errors.New("iperf3 options: hosts must not contain an empty host")
		}
	}
	if o.Host == "" && len(o.Hosts) == 1 {
		o.Host = o.Hosts[0]
	}
	if o.Host == "" && len(o.Hosts) == 0 {
		return Options{}, errors.New("iperf3 options: host is required")
	}
	if o.Port == 0 {
		o.Port = 5201
	}
	if o.Port < 1 || o.Port > 65535 {
		return Options{}, errors.New("iperf3 options: port out of range")
	}
	if o.PortRangeEnd != 0 {
		if o.PortRangeEnd < o.Port {
			return Options{}, errors.New("iperf3 options: port_range_end must be >= port")
		}
		if o.PortRangeEnd > 65535 {
			return Options{}, errors.New("iperf3 options: port_range_end out of range")
		}
	}
	if o.Protocol == "" {
		o.Protocol = "tcp"
	}
	if o.Protocol != "tcp" && o.Protocol != "udp" {
		return Options{}, fmt.Errorf("iperf3 options: protocol %q must be tcp or udp", o.Protocol)
	}
	if o.Parallel == 0 {
		o.Parallel = 1
	}
	if o.Parallel < 1 {
		return Options{}, errors.New("iperf3 options: parallel must be >= 1")
	}
	if o.DurationS == 0 {
		o.DurationS = 10
	}
	if o.DurationS < 1 {
		return Options{}, errors.New("iperf3 options: duration_s must be >= 1")
	}
	if o.Reverse && o.Bidir {
		return Options{}, errors.New("iperf3 options: reverse and bidir are mutually exclusive")
	}
	if o.Bidir && o.Protocol == "udp" {
		return Options{}, errors.New("iperf3 options: bidir is not supported with udp")
	}
	if o.UDPBitrate != "" && o.Protocol != "udp" {
		return Options{}, errors.New("iperf3 options: udp_bitrate requires protocol udp")
	}
	if o.Password != "" && (o.Username == "" || o.RSAPublicKeyPath == "") {
		return Options{}, errors.New("iperf3 options: password requires username and rsa_public_key_path")
	}
	return o, nil
}

// buildArgs renders the iperf3 client arguments. stream selects
// --json-stream (iperf3 >= 3.17) over the single -J summary document.
func buildArgs(o Options, stream bool) []string {
	args := []string{"-c", o.Host, "-p", strconv.Itoa(o.Port)}
	if stream {
		args = append(args, "--json-stream")
	} else {
		args = append(args, "-J")
	}
	args = append(args, "-t", strconv.Itoa(o.DurationS), "-P", strconv.Itoa(o.Parallel))
	if o.Reverse {
		args = append(args, "-R")
	}
	if o.Bidir {
		args = append(args, "--bidir")
	}
	if o.Protocol == "udp" {
		args = append(args, "-u")
		if o.UDPBitrate != "" {
			args = append(args, "-b", o.UDPBitrate)
		}
	}
	if o.Bind != "" {
		args = append(args, "-B", o.Bind)
	}
	if o.Username != "" {
		args = append(args, "--username", o.Username)
	}
	if o.RSAPublicKeyPath != "" {
		args = append(args, "--rsa-public-key-path", o.RSAPublicKeyPath)
	}
	return args
}
