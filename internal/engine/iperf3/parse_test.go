package iperf3

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/metril/speedtest-tracker/internal/engine"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return b
}

func mustOptions(t *testing.T, raw string) Options {
	t.Helper()
	o, err := parseOptions(json.RawMessage(raw))
	if err != nil {
		t.Fatalf("parseOptions: %v", err)
	}
	return o
}

func TestParseSummaryTCP(t *testing.T) {
	res, err := parseSummary(fixture(t, "tcp.json"), mustOptions(t, `{"host":"nas.lan"}`))
	if err != nil {
		t.Fatalf("parseSummary: %v", err)
	}
	// Forward TCP: the client sends, so sum_received is the upload.
	if res.UploadBps != 100_000_000 {
		t.Errorf("UploadBps = %v, want 100000000 (sum_received, not sum_sent)", res.UploadBps)
	}
	if res.DownloadBps != 0 {
		t.Errorf("DownloadBps = %v, want 0", res.DownloadBps)
	}
	if res.BytesUp != 125_000_000 {
		t.Errorf("BytesUp = %d", res.BytesUp)
	}
	if res.ServerHost != "nas.lan:5201" {
		t.Errorf("ServerHost = %q", res.ServerHost)
	}
	if len(res.Raw) == 0 {
		t.Error("Raw is empty")
	}
}

func TestParseSummaryTCPReverse(t *testing.T) {
	res, err := parseSummary(fixture(t, "tcp_reverse.json"), mustOptions(t, `{"host":"nas.lan","reverse":true}`))
	if err != nil {
		t.Fatalf("parseSummary: %v", err)
	}
	if res.DownloadBps != 752_000_000 {
		t.Errorf("DownloadBps = %v, want 752000000", res.DownloadBps)
	}
	if res.UploadBps != 0 {
		t.Errorf("UploadBps = %v, want 0", res.UploadBps)
	}
	if res.BytesDown != 940_000_000 {
		t.Errorf("BytesDown = %d", res.BytesDown)
	}
}

func TestParseSummaryBidir(t *testing.T) {
	res, err := parseSummary(fixture(t, "bidir.json"), mustOptions(t, `{"host":"nas.lan","bidir":true}`))
	if err != nil {
		t.Fatalf("parseSummary: %v", err)
	}
	if res.UploadBps != 100_000_000 {
		t.Errorf("UploadBps = %v, want sum_bidir_forward 100000000", res.UploadBps)
	}
	if res.DownloadBps != 200_000_000 {
		t.Errorf("DownloadBps = %v, want sum_bidir_reverse 200000000", res.DownloadBps)
	}
	if res.BytesUp != 125_000_000 || res.BytesDown != 250_000_000 {
		t.Errorf("bytes = %d/%d", res.BytesUp, res.BytesDown)
	}
}

func TestParseSummaryUDP(t *testing.T) {
	res, err := parseSummary(fixture(t, "udp.json"), mustOptions(t, `{"host":"nas.lan","protocol":"udp","udp_bitrate":"100M"}`))
	if err != nil {
		t.Fatalf("parseSummary: %v", err)
	}
	if res.UploadBps != 100_000_000 {
		t.Errorf("UploadBps = %v", res.UploadBps)
	}
	if res.JitterMs != 0.842 {
		t.Errorf("JitterMs = %v, want 0.842", res.JitterMs)
	}
	if res.PacketLossPct != 0.013 {
		t.Errorf("PacketLossPct = %v, want 0.013", res.PacketLossPct)
	}
}

func TestParseSummaryUDPReverse(t *testing.T) {
	res, err := parseSummary(fixture(t, "udp_reverse.json"), mustOptions(t, `{"host":"nas.lan","protocol":"udp","reverse":true}`))
	if err != nil {
		t.Fatalf("parseSummary: %v", err)
	}
	if res.DownloadBps != 94_400_000 {
		t.Errorf("DownloadBps = %v, want 94400000", res.DownloadBps)
	}
	if res.UploadBps != 0 {
		t.Errorf("UploadBps = %v, want 0", res.UploadBps)
	}
	if res.JitterMs != 0.951 {
		t.Errorf("JitterMs = %v, want 0.951", res.JitterMs)
	}
	if res.PacketLossPct != 0.024 {
		t.Errorf("PacketLossPct = %v, want 0.024", res.PacketLossPct)
	}
}

func TestParseSummaryError(t *testing.T) {
	_, err := parseSummary(fixture(t, "error.json"), mustOptions(t, `{"host":"nas.lan"}`))
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "Connection refused") {
		t.Errorf("err = %v", err)
	}
}

func TestParseStreamJSONL(t *testing.T) {
	f, err := os.Open("testdata/stream_tcp.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var events []engine.Progress
	res, err := parseStreamJSONL(f, mustOptions(t, `{"host":"nas.lan","duration_s":2}`), func(p engine.Progress) {
		events = append(events, p)
	})
	if err != nil {
		t.Fatalf("parseStreamJSONL: %v", err)
	}
	if res.UploadBps != 100_000_000 {
		t.Errorf("UploadBps = %v", res.UploadBps)
	}
	if len(events) < 3 {
		t.Fatalf("got %d events, want at least 3", len(events))
	}
	if events[0].Phase != engine.PhaseConnecting {
		t.Errorf("events[0] = %+v", events[0])
	}
	if events[1].Phase != engine.PhaseUpload || events[1].Bps != 100_000_000 || events[1].ElapsedMs != 1000 {
		t.Errorf("events[1] = %+v", events[1])
	}
	if events[1].Progress != 0.5 {
		t.Errorf("progress = %v, want 0.5 (1s of a 2s test)", events[1].Progress)
	}
	if last := events[len(events)-1]; last.Phase != engine.PhaseDone {
		t.Errorf("last = %+v", last)
	}
}
