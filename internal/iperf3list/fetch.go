// Package iperf3list fetches and caches the export.iperf3serverlist.net
// public server list: Fetch parses one snapshot, Refresher keeps the
// cache (internal/store's iperf3_servers table) refreshed on a daily
// schedule plus on-demand.
package iperf3list

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/metril/speedtest-tracker/internal/store"
)

// fetchTimeout bounds one Fetch call.
const fetchTimeout = 30 * time.Second

// maxResponseBytes caps how much of the outbound server-list body is
// read, guarding against a misbehaving or malicious upstream streaming an
// unbounded body.
const maxResponseBytes = 32 << 20 // 32 MiB

// rejectCrossHostRedirect is a http.Client CheckRedirect func that refuses
// to follow a redirect whose target host differs from the original
// request's host.
func rejectCrossHostRedirect(req *http.Request, via []*http.Request) error {
	if len(via) == 0 {
		return nil
	}
	if req.URL.Host != via[0].URL.Host {
		return fmt.Errorf("refusing redirect from %s to different host %s", via[0].URL.Host, req.URL.Host)
	}
	return nil
}

// rawServer mirrors one row of export.iperf3serverlist.net's JSON, whose
// keys are verified against the live feed (see the task brief).
type rawServer struct {
	Host      string `json:"IP/HOST"`
	Port      string `json:"PORT"`
	Options   string `json:"OPTIONS"`
	GBs       string `json:"GB/S"`
	Continent string `json:"CONTINENT"`
	Country   string `json:"COUNTRY"`
	Site      string `json:"SITE"`
	Provider  string `json:"PROVIDER"`
}

// Fetch retrieves and parses url (export.iperf3serverlist.net's JSON
// export) using client. Each row is parsed defensively: a row with an
// empty host or an unparsable PORT is skipped rather than failing the
// whole batch. An error is returned only for a transport failure, a
// non-200 response, or a body that isn't a JSON array at all.
func Fetch(ctx context.Context, client *http.Client, url string) ([]store.Iperf3Server, error) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build iperf3 server list request: %w", err)
	}

	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch iperf3 server list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch iperf3 server list: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read iperf3 server list: %w", err)
	}
	if len(body) > maxResponseBytes {
		return nil, fmt.Errorf("iperf3 server list response exceeds %d bytes", maxResponseBytes)
	}

	var raw []rawServer
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode iperf3 server list: %w", err)
	}

	out := make([]store.Iperf3Server, 0, len(raw))
	for _, r := range raw {
		host := strings.TrimSpace(r.Host)
		if host == "" {
			continue
		}
		port, portEnd, ok := parsePortRange(r.Port)
		if !ok {
			continue
		}
		reverse, udp, ipv6 := parseOptions(r.Options)
		out = append(out, store.Iperf3Server{
			Host:            host,
			Port:            port,
			PortEnd:         portEnd,
			Options:         r.Options,
			SupportsReverse: reverse,
			SupportsUDP:     udp,
			SupportsIPv6:    ipv6,
			GBs:             r.GBs,
			Continent:       r.Continent,
			Country:         r.Country,
			Site:            r.Site,
			Provider:        r.Provider,
		})
	}
	return out, nil
}

// parsePortRange parses a PORT field, which is either a single port
// ("5201") or a range ("9205-9240"). For a single port, end is 0 (no
// range). It reports false for empty, unparsable or non-positive values.
func parsePortRange(raw string) (start, end int, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, 0, false
	}
	first := raw
	var last string
	if idx := strings.Index(raw, "-"); idx > 0 {
		first = raw[:idx]
		last = raw[idx+1:]
	}
	n, err := strconv.Atoi(strings.TrimSpace(first))
	if err != nil || n <= 0 {
		return 0, 0, false
	}
	if last == "" {
		return n, 0, true
	}
	e, err := strconv.Atoi(strings.TrimSpace(last))
	if err != nil || e <= 0 || e < n || e > 65535 {
		// A malformed, out-of-order or out-of-range end still leaves the
		// start port usable — just fall back to a single port.
		return n, 0, true
	}
	return n, e, true
}

// parseOptions parses a comma-separated OPTIONS field (e.g. "-R,-u,-6")
// into whether the server supports reverse mode, UDP and IPv6.
func parseOptions(raw string) (reverse, udp, ipv6 bool) {
	for _, part := range strings.Split(raw, ",") {
		switch strings.ToLower(strings.TrimSpace(part)) {
		case "-r":
			reverse = true
		case "-u":
			udp = true
		case "-6":
			ipv6 = true
		}
	}
	return reverse, udp, ipv6
}
