package iperf3

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/metril/speedtest-tracker/internal/engine"
	"github.com/metril/speedtest-tracker/internal/engine/exectest"
)

func abs(t *testing.T, name string) string {
	t.Helper()
	p, err := filepath.Abs("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRunSummaryMode(t *testing.T) {
	// --version reports 3.12, so the engine must use -J, not --json-stream.
	body := `case "$1" in
  --version) echo "iperf 3.12 (cJSON 1.7.15)"; exit 0;;
esac
for a in "$@"; do
  if [ "$a" = "--json-stream" ]; then echo "unknown option" >&2; exit 1; fi
done
cat ` + "\"" + abs(t, "tcp.json") + "\"" + `
exit 0`
	bin := exectest.Build(t, "iperf3", body)

	res, err := New(bin).Run(context.Background(), json.RawMessage(`{"host":"nas.lan"}`), nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.UploadBps != 100_000_000 {
		t.Errorf("UploadBps = %v", res.UploadBps)
	}
}

func TestRunStreamMode(t *testing.T) {
	body := `case "$1" in
  --version) echo "iperf 3.17.1 (cJSON 1.7.15)"; exit 0;;
esac
cat ` + "\"" + abs(t, "stream_tcp.jsonl") + "\"" + `
exit 0`
	bin := exectest.Build(t, "iperf3", body)

	var n int
	res, err := New(bin).Run(context.Background(), json.RawMessage(`{"host":"nas.lan","duration_s":2}`), func(engine.Progress) { n++ })
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.UploadBps != 100_000_000 {
		t.Errorf("UploadBps = %v", res.UploadBps)
	}
	if n < 3 {
		t.Errorf("progress events = %d, want >= 3", n)
	}
}

func TestRunErrorJSONBeatsExitCode(t *testing.T) {
	body := `case "$1" in
  --version) echo "iperf 3.12"; exit 0;;
esac
cat ` + "\"" + abs(t, "error.json") + "\"" + `
exit 1`
	bin := exectest.Build(t, "iperf3", body)

	_, err := New(bin).Run(context.Background(), json.RawMessage(`{"host":"nas.lan"}`), nil)
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "Connection refused") {
		t.Errorf("err = %v, want the JSON error message not the exit status", err)
	}
}

func TestRunRejectsBadOptions(t *testing.T) {
	if _, err := New("iperf3").Run(context.Background(), json.RawMessage(`{}`), nil); err == nil {
		t.Fatal("want error for missing host")
	}
}

func TestNameAndValidate(t *testing.T) {
	e := New("iperf3")
	if e.Name() != "iperf3" {
		t.Errorf("Name = %q", e.Name())
	}
	if err := e.Validate(json.RawMessage(`{"host":"h"}`)); err != nil {
		t.Errorf("Validate: %v", err)
	}
	if err := e.Validate(json.RawMessage(`{}`)); err == nil {
		t.Error("want error")
	}
}
