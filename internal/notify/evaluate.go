// Package notify evaluates completed results against per-target and global
// thresholds and delivers alert and recovery notifications to the
// configured webhook and apprise channels. Evaluation is pure; all
// delivery happens on the Notifier's own goroutine so the runner is never
// blocked.
package notify

import (
	"bytes"
	"encoding/json"

	"github.com/metril/speedtest-tracker/internal/settings"
	"github.com/metril/speedtest-tracker/internal/store"
)

// Metric names one alertable quantity. The value doubles as the `metric`
// column of notification_state, so these strings are persisted: do not
// rename them without a migration.
type Metric string

const (
	MetricDownload Metric = "download"
	MetricUpload   Metric = "upload"
	MetricPing     Metric = "ping"
	MetricJitter   Metric = "jitter"
	MetricLoss     Metric = "loss"
	MetricFailure  Metric = "failure"
)

// Eval is the outcome of checking one metric against its limit.
type Eval struct {
	Metric   Metric
	Breached bool
	Value    float64
	Limit    float64
	Unit     string
}

// ParseThresholds decodes raw into a Thresholds value plus the set of JSON
// field names that were explicitly set to null (as opposed to merely
// absent). A per-target override document uses this distinction: an absent
// key inherits the global default, while an explicit null (e.g.
// {"ping_ms_max": null}) disables that metric for the target regardless of
// the default. Empty, whitespace-only or top-level "null" input returns the
// zero value, a nil set and a nil error.
func ParseThresholds(raw json.RawMessage) (settings.Thresholds, map[string]bool, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return settings.Thresholds{}, nil, nil
	}
	var t settings.Thresholds
	if err := json.Unmarshal(trimmed, &t); err != nil {
		return settings.Thresholds{}, nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &fields); err != nil {
		return settings.Thresholds{}, nil, err
	}
	var nulled map[string]bool
	for k, v := range fields {
		if bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			if nulled == nil {
				nulled = map[string]bool{}
			}
			nulled[k] = true
		}
	}
	return t, nulled, nil
}

// nullableFields clears one Thresholds field, keyed by its JSON name. Only
// the fields with per-target "disable" semantics are included; the SLA
// fields keep their existing "null means inherit" behavior and are handled
// separately (see internal/store/summary.go's resolvePlan).
var nullableFields = map[string]func(*settings.Thresholds){
	"download_mbps_min": func(t *settings.Thresholds) { t.DownloadMbpsMin = nil },
	"upload_mbps_min":   func(t *settings.Thresholds) { t.UploadMbpsMin = nil },
	"ping_ms_max":       func(t *settings.Thresholds) { t.PingMsMax = nil },
	"jitter_ms_max":     func(t *settings.Thresholds) { t.JitterMsMax = nil },
	"loss_pct_max":      func(t *settings.Thresholds) { t.LossPctMax = nil },
	"notify_on_failure": func(t *settings.Thresholds) { t.NotifyOnFailure = nil },
	"notify_always":     func(t *settings.Thresholds) { t.NotifyAlways = nil },
}

// Merge layers override on top of base: any field set (non-nil) in override
// replaces the corresponding base field, unset fields fall through to base,
// and any field named in nulled (from ParseThresholds) is forced to nil
// regardless of base, disabling that metric for the target. Neither base
// nor override is mutated.
func Merge(base, override settings.Thresholds, nulled map[string]bool) settings.Thresholds {
	merged := base
	if override.DownloadMbpsMin != nil {
		merged.DownloadMbpsMin = override.DownloadMbpsMin
	}
	if override.UploadMbpsMin != nil {
		merged.UploadMbpsMin = override.UploadMbpsMin
	}
	if override.PingMsMax != nil {
		merged.PingMsMax = override.PingMsMax
	}
	if override.JitterMsMax != nil {
		merged.JitterMsMax = override.JitterMsMax
	}
	if override.LossPctMax != nil {
		merged.LossPctMax = override.LossPctMax
	}
	if override.NotifyOnFailure != nil {
		merged.NotifyOnFailure = override.NotifyOnFailure
	}
	if override.NotifyAlways != nil {
		merged.NotifyAlways = override.NotifyAlways
	}
	for k := range nulled {
		if clear, ok := nullableFields[k]; ok {
			clear(&merged)
		}
	}
	return merged
}

// Evaluate checks res against t and returns one Eval per set threshold, in
// a stable order: download, upload, ping, jitter, loss, failure.
//
// A failed result (res.Status != "ok") carries zero download/ping/loss
// values, so evaluating those thresholds against it would fire every metric
// at once; instead, when NotifyOnFailure is set and true, Evaluate returns
// a single breached MetricFailure eval and nothing else.
//
// For an "ok" result, Evaluate emits a non-breached MetricFailure eval when
// NotifyOnFailure is true (so a standing failure alert can recover), plus
// one eval per other threshold that is set.
func Evaluate(res *store.Result, t settings.Thresholds) []Eval {
	var evals []Eval

	if res.Status != "ok" {
		if t.NotifyOnFailure != nil && *t.NotifyOnFailure {
			evals = append(evals, Eval{Metric: MetricFailure, Breached: true})
		}
		return evals
	}

	if t.DownloadMbpsMin != nil {
		v := res.DownloadBps / 1e6
		evals = append(evals, Eval{Metric: MetricDownload, Breached: v < *t.DownloadMbpsMin,
			Value: v, Limit: *t.DownloadMbpsMin, Unit: "Mbps"})
	}
	if t.UploadMbpsMin != nil {
		v := res.UploadBps / 1e6
		evals = append(evals, Eval{Metric: MetricUpload, Breached: v < *t.UploadMbpsMin,
			Value: v, Limit: *t.UploadMbpsMin, Unit: "Mbps"})
	}
	if t.PingMsMax != nil {
		v := res.PingMs
		evals = append(evals, Eval{Metric: MetricPing, Breached: v > *t.PingMsMax,
			Value: v, Limit: *t.PingMsMax, Unit: "ms"})
	}
	if t.JitterMsMax != nil {
		v := res.JitterMs
		evals = append(evals, Eval{Metric: MetricJitter, Breached: v > *t.JitterMsMax,
			Value: v, Limit: *t.JitterMsMax, Unit: "ms"})
	}
	if t.LossPctMax != nil {
		v := res.PacketLossPct
		evals = append(evals, Eval{Metric: MetricLoss, Breached: v > *t.LossPctMax,
			Value: v, Limit: *t.LossPctMax, Unit: "%"})
	}
	if t.NotifyOnFailure != nil && *t.NotifyOnFailure {
		evals = append(evals, Eval{Metric: MetricFailure, Breached: false})
	}

	return evals
}
