// Package metrics owns the process's Prometheus registry: latest-per-target
// gauges fed by the runner's result sink, runner queue depth, run counts,
// summary-cache hit rate and the VictoriaMetrics push counters.
package metrics

import (
	"net/http"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/metril/speedtest-tracker/internal/store"
)

const ns = "speedtest"

var latestLabels = []string{"target", "target_id", "engine", "schedule"}

// Registry owns the process's Prometheus collectors and serves them over
// HTTP. Create one with New per process.
type Registry struct {
	reg *prometheus.Registry

	latestDownload *prometheus.GaugeVec
	latestUpload   *prometheus.GaugeVec
	latestPing     *prometheus.GaugeVec
	latestJitter   *prometheus.GaugeVec
	latestLoss     *prometheus.GaugeVec
	runsTotal      *prometheus.CounterVec
	summaryHits    prometheus.Counter
	summaryMisses  prometheus.Counter
}

// New creates a Registry and registers its built-in collectors. Call once
// per process.
func New() *Registry {
	m := &Registry{
		reg: prometheus.NewRegistry(),
		latestDownload: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns, Name: "latest_download_bps", Help: "Most recent download throughput in bits per second, per target.",
		}, latestLabels),
		latestUpload: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns, Name: "latest_upload_bps", Help: "Most recent upload throughput in bits per second, per target.",
		}, latestLabels),
		latestPing: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns, Name: "latest_ping_ms", Help: "Most recent ping latency in milliseconds, per target.",
		}, latestLabels),
		latestJitter: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns, Name: "latest_jitter_ms", Help: "Most recent jitter in milliseconds, per target.",
		}, latestLabels),
		latestLoss: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns, Name: "latest_packet_loss_pct", Help: "Most recent packet loss percentage, per target.",
		}, latestLabels),
		runsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Name: "runs_total", Help: "Total number of speed test runs, by status.",
		}, []string{"status"}),
		summaryHits: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: ns, Name: "summary_cache_hits_total", Help: "Number of /stats/summary requests served from cache.",
		}),
		summaryMisses: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: ns, Name: "summary_cache_misses_total", Help: "Number of /stats/summary requests that recomputed the summary.",
		}),
	}
	m.reg.MustRegister(
		m.latestDownload, m.latestUpload, m.latestPing, m.latestJitter, m.latestLoss,
		m.runsTotal, m.summaryHits, m.summaryMisses,
	)
	return m
}

// Handler serves the Prometheus exposition format for this registry.
func (m *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(m.reg, promhttp.HandlerOpts{})
}

// ObserveResult updates the latest-per-target gauges. A failed result has
// no meaningful throughput numbers, so it only bumps the run counter —
// overwriting the gauges with zeros would make a dashboard read as "0
// Mbps" rather than "last known value, test failed".
func (m *Registry) ObserveResult(res *store.Result, schedule string) {
	status := res.Status
	if status == "" {
		status = "unknown"
	}
	m.runsTotal.WithLabelValues(status).Inc()
	if res.Status == "failed" {
		return
	}
	l := prometheus.Labels{
		"target":    res.TargetName,
		"target_id": targetIDString(res.TargetID),
		"engine":    res.Engine,
		"schedule":  schedule,
	}
	m.latestDownload.With(l).Set(res.DownloadBps)
	m.latestUpload.With(l).Set(res.UploadBps)
	m.latestPing.With(l).Set(res.PingMs)
	m.latestJitter.With(l).Set(res.JitterMs)
	m.latestLoss.With(l).Set(res.PacketLossPct)
}

// SummaryCacheHit records a /stats/summary request served from cache.
func (m *Registry) SummaryCacheHit() { m.summaryHits.Inc() }

// SummaryCacheMiss records a /stats/summary request that recomputed the
// summary.
func (m *Registry) SummaryCacheMiss() { m.summaryMisses.Inc() }

// AddGaugeFunc registers an unlabelled gauge whose value is computed by f
// at scrape time. f may be called concurrently with other scrapes and
// must be safe for that.
func (m *Registry) AddGaugeFunc(name, help string, f func() float64) {
	m.reg.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: name, Help: help}, f))
}

// AddLabelledGaugeFunc registers a gauge with a single label whose values
// are computed by f at scrape time, one metric per map entry. f may be
// called concurrently with other scrapes and must be safe for that.
func (m *Registry) AddLabelledGaugeFunc(name, help, label string, f func() map[string]float64) {
	m.reg.MustRegister(&labelledGaugeFunc{
		desc: prometheus.NewDesc(name, help, []string{label}, nil),
		f:    f,
	})
}

// labelledGaugeFunc is a custom Collector for a single-label gauge whose
// values come from a caller-supplied function evaluated at scrape time.
type labelledGaugeFunc struct {
	desc *prometheus.Desc
	f    func() map[string]float64
}

func (c *labelledGaugeFunc) Describe(ch chan<- *prometheus.Desc) { ch <- c.desc }

func (c *labelledGaugeFunc) Collect(ch chan<- prometheus.Metric) {
	for label, v := range c.f() {
		ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, v, label)
	}
}

func targetIDString(id *int64) string {
	if id == nil {
		return ""
	}
	return strconv.FormatInt(*id, 10)
}
