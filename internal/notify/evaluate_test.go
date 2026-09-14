package notify_test

import (
	"encoding/json"
	"testing"

	"github.com/metril/speedtest-tracker/internal/notify"
	"github.com/metril/speedtest-tracker/internal/settings"
	"github.com/metril/speedtest-tracker/internal/store"
)

func TestMergeOverridesOnlySetFields(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	base := settings.Thresholds{DownloadMbpsMin: f(100), PingMsMax: f(50)}
	over := settings.Thresholds{PingMsMax: f(20)}
	got := notify.Merge(base, over, nil)
	if *got.DownloadMbpsMin != 100 {
		t.Errorf("download inherited = %v, want 100", *got.DownloadMbpsMin)
	}
	if *got.PingMsMax != 20 {
		t.Errorf("ping overridden = %v, want 20", *got.PingMsMax)
	}
	if got.UploadMbpsMin != nil {
		t.Errorf("upload = %v, want nil", got.UploadMbpsMin)
	}
}

func TestMergeNulledFieldDisablesBaseValue(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	base := settings.Thresholds{DownloadMbpsMin: f(100), PingMsMax: f(50)}
	over := settings.Thresholds{PingMsMax: f(20)}
	nulled := map[string]bool{"download_mbps_min": true}

	got := notify.Merge(base, over, nulled)
	if got.DownloadMbpsMin != nil {
		t.Errorf("download = %v, want nil (disabled by null)", got.DownloadMbpsMin)
	}
	if *got.PingMsMax != 20 {
		t.Errorf("ping overridden = %v, want 20", *got.PingMsMax)
	}
}

func TestMergeNulledFieldWinsOverBaseEvenWithoutOverrideValue(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	base := settings.Thresholds{LossPctMax: f(2)}
	nulled := map[string]bool{"loss_pct_max": true}

	got := notify.Merge(base, settings.Thresholds{}, nulled)
	if got.LossPctMax != nil {
		t.Errorf("loss = %v, want nil", got.LossPctMax)
	}
}

func TestEvaluateBreachesAndRecoveries(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	th := settings.Thresholds{DownloadMbpsMin: f(100), UploadMbpsMin: f(10),
		PingMsMax: f(50), JitterMsMax: f(20), LossPctMax: f(2)}
	res := &store.Result{Status: "ok",
		DownloadBps: 50e6, UploadBps: 20e6, PingMs: 80, JitterMs: 5, PacketLossPct: 0}

	got := map[notify.Metric]notify.Eval{}
	for _, e := range notify.Evaluate(res, th) {
		got[e.Metric] = e
	}
	if len(got) != 5 {
		t.Fatalf("evaluated %d metrics, want 5 (failure is not evaluated for an ok result without notify_on_failure)", len(got))
	}
	if !got[notify.MetricDownload].Breached || got[notify.MetricDownload].Value != 50 {
		t.Errorf("download = %+v, want breached at 50 Mbps", got[notify.MetricDownload])
	}
	if got[notify.MetricUpload].Breached {
		t.Errorf("upload = %+v, want ok", got[notify.MetricUpload])
	}
	if !got[notify.MetricPing].Breached {
		t.Errorf("ping = %+v, want breached", got[notify.MetricPing])
	}
	if got[notify.MetricJitter].Breached || got[notify.MetricLoss].Breached {
		t.Errorf("jitter/loss should be ok: %+v %+v", got[notify.MetricJitter], got[notify.MetricLoss])
	}
}

func TestEvaluateUnsetThresholdIsNotEvaluated(t *testing.T) {
	res := &store.Result{Status: "ok", DownloadBps: 1}
	if evals := notify.Evaluate(res, settings.Thresholds{}); len(evals) != 0 {
		t.Fatalf("no thresholds set, got %d evals", len(evals))
	}
}

func TestEvaluateFailedResult(t *testing.T) {
	yes := true
	th := settings.Thresholds{NotifyOnFailure: &yes, PingMsMax: func(v float64) *float64 { return &v }(50)}
	evals := notify.Evaluate(&store.Result{Status: "failed", Error: "boom"}, th)
	if len(evals) != 1 || evals[0].Metric != notify.MetricFailure || !evals[0].Breached {
		t.Fatalf("failed result = %+v, want a single breached failure eval "+
			"(metric thresholds are skipped: a failed result has zero values)", evals)
	}
	ok := notify.Evaluate(&store.Result{Status: "ok", PingMs: 10}, th)
	if len(ok) != 2 {
		t.Fatalf("ok result = %+v, want failure (recovered) plus ping", ok)
	}
	for _, e := range ok {
		if e.Breached {
			t.Errorf("%+v should not be breached", e)
		}
	}
}

func TestParseThresholds(t *testing.T) {
	for _, raw := range []string{"", "{}", "null"} {
		if got, nulled, err := notify.ParseThresholds(json.RawMessage(raw)); err != nil || got.PingMsMax != nil || nulled != nil {
			t.Errorf("ParseThresholds(%q) = %+v, %v, %v; want zero value, nil, nil", raw, got, nulled, err)
		}
	}
	got, nulled, err := notify.ParseThresholds(json.RawMessage(`{"ping_ms_max":25}`))
	if err != nil || got.PingMsMax == nil || *got.PingMsMax != 25 || nulled != nil {
		t.Fatalf("ParseThresholds = %+v, %v, %v", got, nulled, err)
	}
}

func TestEvaluateSkipsMergedNilFromExplicitNull(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	base := settings.Thresholds{PingMsMax: f(50), DownloadMbpsMin: f(100)}
	perTarget, nulled, err := notify.ParseThresholds(json.RawMessage(`{"ping_ms_max":null}`))
	if err != nil {
		t.Fatalf("ParseThresholds error: %v", err)
	}
	th := notify.Merge(base, perTarget, nulled)

	res := &store.Result{Status: "ok", DownloadBps: 200e6, PingMs: 999}
	evals := notify.Evaluate(res, th)
	for _, e := range evals {
		if e.Metric == notify.MetricPing {
			t.Errorf("ping should be disabled (null override), got %+v", e)
		}
	}
	if len(evals) != 1 || evals[0].Metric != notify.MetricDownload {
		t.Fatalf("evals = %+v, want only download (not breached, since it inherits 100)", evals)
	}
}

func TestParseThresholdsDetectsExplicitNull(t *testing.T) {
	got, nulled, err := notify.ParseThresholds(json.RawMessage(
		`{"ping_ms_max":null,"download_mbps_min":50,"notify_on_failure":null}`))
	if err != nil {
		t.Fatalf("ParseThresholds error: %v", err)
	}
	if got.DownloadMbpsMin == nil || *got.DownloadMbpsMin != 50 {
		t.Errorf("download = %v, want 50", got.DownloadMbpsMin)
	}
	if got.PingMsMax != nil {
		t.Errorf("ping = %v, want nil (absent from Thresholds, tracked in nulled instead)", got.PingMsMax)
	}
	if !nulled["ping_ms_max"] || !nulled["notify_on_failure"] {
		t.Errorf("nulled = %v, want ping_ms_max and notify_on_failure set", nulled)
	}
	if nulled["download_mbps_min"] {
		t.Errorf("nulled = %v, download_mbps_min should not be marked null", nulled)
	}
}
