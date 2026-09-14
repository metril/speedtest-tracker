package metrics_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/metril/speedtest-tracker/internal/metrics"
	"github.com/metril/speedtest-tracker/internal/store"
)

func scrape(t *testing.T, m *metrics.Registry) string {
	t.Helper()
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	return rec.Body.String()
}

func TestObserveResultUpdatesLatestGauges(t *testing.T) {
	m := metrics.New()
	id := int64(3)
	m.ObserveResult(&store.Result{
		TargetID: &id, TargetName: "wan", Engine: "ookla", Status: "ok",
		DownloadBps: 1e9, UploadBps: 5e8, PingMs: 4.5,
	}, "nightly")
	body := scrape(t, m)
	for _, want := range []string{
		`speedtest_latest_download_bps{engine="ookla",schedule="nightly",target="wan",target_id="3"} 1e+09`,
		`speedtest_latest_ping_ms{engine="ookla",schedule="nightly",target="wan",target_id="3"} 4.5`,
		`speedtest_runs_total{status="ok"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in:\n%s", want, body)
		}
	}
}

func TestFailedResultDoesNotOverwriteLatestThroughput(t *testing.T) {
	m := metrics.New()
	id := int64(3)
	m.ObserveResult(&store.Result{TargetID: &id, TargetName: "wan", Engine: "ookla", Status: "ok", DownloadBps: 1e9}, "")
	m.ObserveResult(&store.Result{TargetID: &id, TargetName: "wan", Engine: "ookla", Status: "failed"}, "")
	body := scrape(t, m)
	if !strings.Contains(body, `speedtest_latest_download_bps{engine="ookla",schedule="",target="wan",target_id="3"} 1e+09`) {
		t.Fatalf("a failed result zeroed the last known throughput:\n%s", body)
	}
	if !strings.Contains(body, `speedtest_runs_total{status="failed"} 1`) {
		t.Fatalf("failed run not counted:\n%s", body)
	}
}

func TestSummaryCacheCountersAndGaugeFuncs(t *testing.T) {
	m := metrics.New()
	m.SummaryCacheHit()
	m.SummaryCacheHit()
	m.SummaryCacheMiss()
	m.AddGaugeFunc("speedtest_vm_push_pushed_total", "batches pushed", func() float64 { return 12 })
	m.AddLabelledGaugeFunc("speedtest_runner_queue_depth", "queued jobs", "lane",
		func() map[string]float64 { return map[string]float64{"wan": 2} })
	body := scrape(t, m)
	for _, want := range []string{
		"speedtest_summary_cache_hits_total 2",
		"speedtest_summary_cache_misses_total 1",
		"speedtest_vm_push_pushed_total 12",
		`speedtest_runner_queue_depth{lane="wan"} 2`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in:\n%s", want, body)
		}
	}
}
