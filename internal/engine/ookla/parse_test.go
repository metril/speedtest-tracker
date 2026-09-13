package ookla

import (
	"os"
	"strings"
	"testing"

	"github.com/metril/speedtest-tracker/internal/engine"
)

func openFixture(t *testing.T, name string) *os.File {
	t.Helper()
	f, err := os.Open("testdata/" + name)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

func TestParseStreamSuccess(t *testing.T) {
	var events []engine.Progress
	res, err := parseStream(openFixture(t, "tcp_success.jsonl"), func(p engine.Progress) {
		events = append(events, p)
	})
	if err != nil {
		t.Fatalf("parseStream: %v", err)
	}
	// bandwidth is bytes/s and must be multiplied by 8.
	if res.DownloadBps != 92_000_000 {
		t.Errorf("DownloadBps = %v, want 92000000", res.DownloadBps)
	}
	if res.UploadBps != 20_000_000 {
		t.Errorf("UploadBps = %v, want 20000000", res.UploadBps)
	}
	if res.PingMs != 12.3 || res.JitterMs != 1.2 || res.PacketLossPct != 0 {
		t.Errorf("ping/jitter/loss = %v/%v/%v", res.PingMs, res.JitterMs, res.PacketLossPct)
	}
	if res.BytesDown != 34_500_000 || res.BytesUp != 7_500_000 {
		t.Errorf("bytes = %d/%d", res.BytesDown, res.BytesUp)
	}
	if res.ServerID != "12345" || res.ServerName != "Example Server" || res.ServerHost != "speed.example.net" {
		t.Errorf("server = %q/%q/%q", res.ServerID, res.ServerName, res.ServerHost)
	}
	if res.ISP != "Example ISP" || res.ExternalIP != "203.0.113.7" {
		t.Errorf("isp/ip = %q/%q", res.ISP, res.ExternalIP)
	}
	if res.ResultURL != "https://www.speedtest.net/result/c/9f0d0b3e-1111-2222-3333-444455556666" {
		t.Errorf("ResultURL = %q", res.ResultURL)
	}
	if len(res.Raw) == 0 || !strings.Contains(string(res.Raw), `"type":"result"`) {
		t.Errorf("Raw = %s", res.Raw)
	}

	// Implementation also emits a PhaseConnecting event for the testStart
	// line, so there are 8 events total (not 7): connecting, ping x2,
	// download x2, upload x2, done.
	if len(events) != 8 {
		t.Fatalf("got %d progress events, want 8", len(events))
	}
	if events[0].Phase != engine.PhaseConnecting {
		t.Errorf("events[0] = %+v, want PhaseConnecting", events[0])
	}
	if events[1].Phase != engine.PhasePing || events[1].PingMs != 11.8 || events[1].Progress != 0.5 {
		t.Errorf("events[1] = %+v", events[1])
	}
	if events[3].Phase != engine.PhaseDownload || events[3].Bps != 40_000_000 || events[3].ElapsedMs != 500 {
		t.Errorf("events[3] = %+v", events[3])
	}
	if events[5].Phase != engine.PhaseUpload || events[5].Bps != 9_600_000 {
		t.Errorf("events[5] = %+v", events[5])
	}
	if events[0].ServerName != "Example Server" {
		t.Errorf("server name not carried from testStart: %+v", events[0])
	}
	if last := events[len(events)-1]; last.Phase != engine.PhaseDone || last.Progress != 1 {
		t.Errorf("last event = %+v", last)
	}
}

func TestParseStreamNullPacketLoss(t *testing.T) {
	res, err := parseStream(openFixture(t, "null_packetloss.jsonl"), nil)
	if err != nil {
		t.Fatalf("parseStream: %v", err)
	}
	if res.PacketLossPct != 0 {
		t.Errorf("PacketLossPct = %v, want 0", res.PacketLossPct)
	}
}

func TestParseStreamErrorLog(t *testing.T) {
	var events []engine.Progress
	_, err := parseStream(openFixture(t, "error_dns.jsonl"), func(p engine.Progress) { events = append(events, p) })
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "Temporary failure in name resolution") {
		t.Errorf("err = %v", err)
	}
	if len(events) != 1 || events[0].Phase != engine.PhaseError {
		t.Errorf("events = %+v", events)
	}
}

func TestParseStreamIgnoresGarbageLines(t *testing.T) {
	in := strings.NewReader("not json\n\n" +
		`{"type":"ping","ping":{"jitter":1,"latency":10,"progress":1}}` + "\n" +
		`{"type":"result","ping":{"jitter":1,"latency":10},"download":{"bandwidth":1250000,"bytes":1,"elapsed":1},"upload":{"bandwidth":125000,"bytes":1,"elapsed":1}}` + "\n")
	res, err := parseStream(in, nil)
	if err != nil {
		t.Fatalf("parseStream: %v", err)
	}
	if res.DownloadBps != 10_000_000 {
		t.Errorf("DownloadBps = %v", res.DownloadBps)
	}
}

func TestParseStreamNoResultLine(t *testing.T) {
	if _, err := parseStream(strings.NewReader(""), nil); err == nil {
		t.Fatal("want error for empty stream")
	}
}
