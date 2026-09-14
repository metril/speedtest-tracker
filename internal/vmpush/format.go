// Package vmpush pushes results to VictoriaMetrics using the Prometheus
// text import format with explicit millisecond timestamps, so a
// re-executed or backfilled result lands at the time it actually ran.
package vmpush

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/metril/speedtest-tracker/internal/store"
)

// Meta carries fields not present on store.Result but known to the caller
// at push time.
type Meta struct {
	Schedule string
}

// labelEscaper escapes the three characters the Prometheus text format
// treats specially inside a label value.
var labelEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)

var validLabelName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// builtinLabels are the fixed label keys buildLabels always emits; an
// extra label with a colliding key is skipped so it can never shadow one.
var builtinLabels = map[string]bool{
	"target": true, "target_id": true, "engine": true,
	"server_id": true, "server_name": true, "isp": true, "schedule": true,
}

// Format renders one result as Prometheus import lines. Throughput and
// latency series are emitted only for a successful result; a failed one
// still pushes speedtest_run_success 0 and speedtest_duration_ms so gaps
// are visible in a dashboard. extra labels are appended last, so they can
// never shadow the built-ins.
func Format(res *store.Result, meta Meta, extra map[string]string) []byte {
	ts := timestampMillis(res.StartedAt)
	labels := buildLabels(res, meta, extra)
	var b strings.Builder
	line := func(name string, v float64) {
		fmt.Fprintf(&b, "%s%s %s %d\n", name, labels, strconv.FormatFloat(v, 'g', -1, 64), ts)
	}
	if res.Status != "failed" {
		line("speedtest_download_bps", res.DownloadBps)
		line("speedtest_upload_bps", res.UploadBps)
		line("speedtest_ping_ms", res.PingMs)
		line("speedtest_jitter_ms", res.JitterMs)
		line("speedtest_packet_loss_pct", res.PacketLossPct)
	}
	success := 0.0
	if res.Status == "ok" {
		success = 1
	}
	line("speedtest_run_success", success)
	line("speedtest_duration_ms", float64(res.DurationMs))
	return []byte(b.String())
}

// buildLabels renders the "{k=\"v\",...}" label block: built-ins first in a
// fixed order, then extra keys sorted for deterministic output.
func buildLabels(res *store.Result, meta Meta, extra map[string]string) string {
	targetID := ""
	if res.TargetID != nil {
		targetID = strconv.FormatInt(*res.TargetID, 10)
	}
	var b strings.Builder
	b.WriteByte('{')
	kv := func(k, v string) {
		fmt.Fprintf(&b, "%s=\"%s\",", k, labelEscaper.Replace(v))
	}
	kv("target", res.TargetName)
	kv("target_id", targetID)
	kv("engine", res.Engine)
	kv("server_id", res.ServerID)
	kv("server_name", res.ServerName)
	kv("isp", res.ISP)
	kv("schedule", meta.Schedule)

	keys := make([]string, 0, len(extra))
	for k := range extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if builtinLabels[k] || !validLabelName.MatchString(k) {
			continue
		}
		kv(k, extra[k])
	}

	s := b.String()
	s = strings.TrimSuffix(s, ",")
	return s + "}"
}

// timestampMillis parses StartedAt, falling back to time.Now() when the
// value is unparseable so a malformed row is still pushed, just without a
// historically accurate timestamp.
func timestampMillis(s string) int64 {
	if t, err := time.Parse("2006-01-02T15:04:05.000Z", s); err == nil {
		return t.UnixMilli()
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UnixMilli()
	}
	return time.Now().UnixMilli()
}
