// Package cloudflare implements a native speed test against the Cloudflare
// speed-test endpoints: /cdn-cgi/trace for latency, /__down for download and
// /__up for upload.
package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/metril/speedtest-tracker/internal/engine"
)

// DefaultBaseURL is the public Cloudflare speed-test host.
const DefaultBaseURL = "https://speed.cloudflare.com"

// Default transfer sizes, in bytes.
var (
	defaultDownloadSizes = []int{1_000_000, 10_000_000, 25_000_000, 100_000_000}
	defaultUploadSizes   = []int{100_000, 1_000_000, 10_000_000}
)

const defaultLatencySamples = 10

// Options are the Cloudflare engine's per-target options.
type Options struct {
	DownloadSizes  []int  `json:"download_sizes,omitempty"`
	UploadSizes    []int  `json:"upload_sizes,omitempty"`
	LatencySamples int    `json:"latency_samples,omitempty"`
	BaseURL        string `json:"base_url,omitempty"`
}

// Engine is the native Cloudflare engine.
type Engine struct {
	client *http.Client
}

// New returns a Cloudflare engine. A nil client uses a 120s-timeout default.
func New(client *http.Client) *Engine {
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}
	return &Engine{client: client}
}

// Name implements engine.Engine.
func (e *Engine) Name() string { return "cloudflare" }

// Validate implements engine.Engine.
func (e *Engine) Validate(opts json.RawMessage) error {
	_, err := parseOptions(opts)
	return err
}

func parseOptions(opts json.RawMessage) (Options, error) {
	var o Options
	if len(opts) > 0 {
		if err := json.Unmarshal(opts, &o); err != nil {
			return Options{}, fmt.Errorf("cloudflare options: %w", err)
		}
	}
	if o.LatencySamples < 0 {
		return Options{}, errors.New("cloudflare options: latency_samples must be >= 0")
	}
	if o.LatencySamples == 0 {
		o.LatencySamples = defaultLatencySamples
	}
	if len(o.DownloadSizes) == 0 {
		o.DownloadSizes = append([]int(nil), defaultDownloadSizes...)
	}
	if len(o.UploadSizes) == 0 {
		o.UploadSizes = append([]int(nil), defaultUploadSizes...)
	}
	for _, s := range append(append([]int(nil), o.DownloadSizes...), o.UploadSizes...) {
		if s <= 0 {
			return Options{}, errors.New("cloudflare options: transfer sizes must be positive")
		}
	}
	if o.BaseURL == "" {
		o.BaseURL = DefaultBaseURL
	}
	u, err := url.Parse(o.BaseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return Options{}, fmt.Errorf("cloudflare options: invalid base_url %q", o.BaseURL)
	}
	o.BaseURL = strings.TrimRight(o.BaseURL, "/")
	return o, nil
}

// Run implements engine.Engine.
func (e *Engine) Run(ctx context.Context, opts json.RawMessage, prog func(engine.Progress)) (*engine.Result, error) {
	o, err := parseOptions(opts)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	elapsed := func() int64 { return time.Since(start).Milliseconds() }

	engine.Emit(prog, engine.Progress{Phase: engine.PhaseConnecting})

	pingMs, jitterMs, colo, ip, err := e.latency(ctx, o, prog, elapsed)
	if err != nil {
		return nil, err
	}

	downBps, bytesDown, err := e.transfer(ctx, o, engine.PhaseDownload, o.DownloadSizes, pingMs, colo, prog, elapsed)
	if err != nil {
		return nil, err
	}
	upBps, bytesUp, err := e.transfer(ctx, o, engine.PhaseUpload, o.UploadSizes, pingMs, colo, prog, elapsed)
	if err != nil {
		return nil, err
	}

	raw, _ := json.Marshal(map[string]any{
		"base_url": o.BaseURL, "colo": colo, "external_ip": ip,
		"download_sizes": o.DownloadSizes, "upload_sizes": o.UploadSizes,
	})
	res := &engine.Result{
		DownloadBps: downBps, UploadBps: upBps,
		PingMs: pingMs, JitterMs: jitterMs,
		BytesDown: bytesDown, BytesUp: bytesUp,
		ServerID: colo, ServerName: colo,
		ServerHost: strings.TrimPrefix(strings.TrimPrefix(o.BaseURL, "https://"), "http://"),
		ExternalIP: ip, Raw: raw,
	}
	engine.Emit(prog, engine.Progress{
		Phase: engine.PhaseDone, Progress: 1, Bps: downBps,
		PingMs: pingMs, JitterMs: jitterMs, ElapsedMs: elapsed(), ServerName: colo,
	})
	return res, nil
}

// latency samples GET /cdn-cgi/trace and returns the minimum round trip, the
// mean absolute difference between consecutive samples as jitter, plus the
// colo and external IP parsed from the last response.
func (e *Engine) latency(ctx context.Context, o Options, prog func(engine.Progress), elapsed func() int64) (float64, float64, string, string, error) {
	var (
		samples  []float64
		colo, ip string
	)
	for i := 0; i < o.LatencySamples; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.BaseURL+"/cdn-cgi/trace", nil)
		if err != nil {
			return 0, 0, "", "", err
		}
		t0 := time.Now()
		resp, err := e.client.Do(req)
		if err != nil {
			return 0, 0, "", "", fmt.Errorf("cloudflare trace: %w", err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		rtt := float64(time.Since(t0).Microseconds()) / 1000
		if err != nil {
			return 0, 0, "", "", err
		}
		if resp.StatusCode != http.StatusOK {
			return 0, 0, "", "", fmt.Errorf("cloudflare trace: status %d", resp.StatusCode)
		}
		for _, l := range strings.Split(string(body), "\n") {
			k, v, ok := strings.Cut(l, "=")
			if !ok {
				continue
			}
			switch k {
			case "colo":
				colo = v
			case "ip":
				ip = v
			}
		}
		samples = append(samples, rtt)
		engine.Emit(prog, engine.Progress{
			Phase: engine.PhasePing, Progress: float64(i+1) / float64(o.LatencySamples),
			PingMs: rtt, ElapsedMs: elapsed(), ServerName: colo,
		})
	}
	if len(samples) == 0 {
		return 0, 0, colo, ip, errors.New("cloudflare: no latency samples")
	}
	min := samples[0]
	for _, s := range samples {
		if s < min {
			min = s
		}
	}
	var jitter float64
	if len(samples) > 1 {
		var sum float64
		for i := 1; i < len(samples); i++ {
			d := samples[i] - samples[i-1]
			if d < 0 {
				d = -d
			}
			sum += d
		}
		jitter = sum / float64(len(samples)-1)
	}
	// A zero RTT (loopback, coarse clock) would make bps infinite later.
	if min <= 0 {
		min = 0.001
	}
	return min, jitter, colo, ip, nil
}

// transfer runs every configured size in phase and returns the p90 of the
// per-request throughput (bits/s, TTFB excluded) and the total bytes moved.
func (e *Engine) transfer(ctx context.Context, o Options, phase engine.Phase, sizes []int, pingMs float64, colo string, prog func(engine.Progress), elapsed func() int64) (float64, int64, error) {
	var (
		rates []float64
		total int64
	)
	for i, size := range sizes {
		var (
			dur time.Duration
			err error
		)
		if phase == engine.PhaseDownload {
			dur, err = e.download(ctx, o.BaseURL, size)
		} else {
			dur, err = e.upload(ctx, o.BaseURL, size)
		}
		if err != nil {
			return 0, 0, err
		}
		// Subtract one min-RTT worth of connection/TTFB overhead.
		secs := dur.Seconds() - pingMs/1000
		if secs <= 0 {
			secs = dur.Seconds()
		}
		if secs <= 0 {
			secs = 1e-6
		}
		bps := float64(size) * 8 / secs
		rates = append(rates, bps)
		total += int64(size)
		engine.Emit(prog, engine.Progress{
			Phase: phase, Progress: float64(i+1) / float64(len(sizes)),
			Bps: bps, PingMs: pingMs, ElapsedMs: elapsed(), ServerName: colo,
		})
	}
	return percentile(rates, 0.9), total, nil
}

func (e *Engine) download(ctx context.Context, base string, size int) (time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/__down?bytes=%d", base, size), nil)
	if err != nil {
		return 0, err
	}
	t0 := time.Now()
	resp, err := e.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("cloudflare download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("cloudflare download: status %d", resp.StatusCode)
	}
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		return 0, fmt.Errorf("cloudflare download: %w", err)
	}
	return time.Since(t0), nil
}

func (e *Engine) upload(ctx context.Context, base string, size int) (time.Duration, error) {
	body := bytes.NewReader(make([]byte, size))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/__up", body)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = int64(size)
	t0 := time.Now()
	resp, err := e.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("cloudflare upload: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("cloudflare upload: status %d", resp.StatusCode)
	}
	return time.Since(t0), nil
}

// percentile returns the nearest-rank p-th percentile of values (p in 0..1).
func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	idx := int(float64(len(sorted)-1) * p)
	if idx < 0 {
		idx = 0
	}
	return sorted[idx]
}
