package vmpush_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/metril/speedtest-tracker/internal/store"
	"github.com/metril/speedtest-tracker/internal/vmpush"
)

func TestFormatEmitsAllSeriesWithMillisecondTimestamp(t *testing.T) {
	id := int64(7)
	res := &store.Result{
		TargetID: &id, TargetName: `home "wan"`, Engine: "ookla", Status: "ok",
		StartedAt: "2026-09-14T10:00:00.000Z", DurationMs: 12345,
		DownloadBps: 1e9, UploadBps: 5e8, PingMs: 4.5, JitterMs: 0.75, PacketLossPct: 0,
		ServerID: "1234", ServerName: "Init7", ISP: "Sunrise",
	}
	out := string(vmpush.Format(res, vmpush.Meta{Schedule: "nightly"}, map[string]string{"host": "pi4"}))
	want := `speedtest_download_bps{target="home \"wan\"",target_id="7",engine="ookla",` +
		`server_id="1234",server_name="Init7",isp="Sunrise",schedule="nightly",host="pi4"} 1e+09 1789380000000`
	if !strings.Contains(out, want) {
		t.Fatalf("missing download line.\ngot:\n%s\nwant line:\n%s", out, want)
	}
	for _, name := range []string{
		"speedtest_upload_bps", "speedtest_ping_ms", "speedtest_jitter_ms",
		"speedtest_packet_loss_pct", "speedtest_run_success", "speedtest_duration_ms",
	} {
		if !strings.Contains(out, name+"{") {
			t.Fatalf("missing series %s in:\n%s", name, out)
		}
	}
	if !strings.HasSuffix(out, "\n") {
		t.Fatal("payload must end with a newline")
	}
}

func TestFormatRunSuccessZeroOnFailure(t *testing.T) {
	res := &store.Result{Status: "failed", StartedAt: "2026-09-14T10:00:00.000Z"}
	out := string(vmpush.Format(res, vmpush.Meta{}, nil))
	if !strings.Contains(out, "speedtest_run_success{") || !regexp.MustCompile(`speedtest_run_success\{[^}]*\} 0 `).MatchString(out) {
		t.Fatalf("failed result must push run_success 0, got:\n%s", out)
	}
	if strings.Contains(out, "speedtest_download_bps") {
		t.Fatalf("a failed result has no throughput numbers to push:\n%s", out)
	}
}

func TestFormatEscapesLabelValues(t *testing.T) {
	res := &store.Result{TargetName: "a\\b\"c\nd", Status: "ok", StartedAt: "2026-09-14T10:00:00.000Z"}
	out := string(vmpush.Format(res, vmpush.Meta{}, nil))
	if !strings.Contains(out, `target="a\\b\"c\nd"`) {
		t.Fatalf("label escaping wrong: %s", out)
	}
}
