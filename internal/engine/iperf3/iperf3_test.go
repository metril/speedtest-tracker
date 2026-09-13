package iperf3

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestRunNoDocumentFoldsInStderrAndExit(t *testing.T) {
	// Nothing on stdout, and the summary probe fails too, so both branches
	// must fall back to reporting the exit error plus the stderr tail.
	bin := exectest.Build(t, "iperf3", "echo boom >&2\nexit 2")
	e := &Engine{Bin: bin, ForceSummary: true}

	_, err := e.Run(context.Background(), json.RawMessage(`{"host":"nas.lan"}`), nil)
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("err = %v, want it to contain stderr output", err)
	}
}

func TestRunHonoursContextCancel(t *testing.T) {
	bin := exectest.Build(t, "iperf3", "sleep 30")
	e := &Engine{Bin: bin, ForceSummary: true}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	done := make(chan error, 1)
	go func() {
		_, err := e.Run(ctx, json.RawMessage(`{"host":"nas.lan"}`), nil)
		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want context.Canceled", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return within 3s of cancellation")
	}
}

func TestRunSetsPasswordEnv(t *testing.T) {
	// Echo the env var to stderr and fail with no stdout, so the value ends
	// up folded into the returned error where the test can see it.
	body := `case "$1" in
  --version) echo "iperf 3.17.1"; exit 0;;
esac
echo "IPERF3_PASSWORD=$IPERF3_PASSWORD" >&2
exit 3`
	bin := exectest.Build(t, "iperf3", body)
	e := &Engine{Bin: bin, ForceSummary: true}

	opts := json.RawMessage(`{"host":"nas.lan","username":"bob","password":"secret","rsa_public_key_path":"/etc/iperf/pub.pem"}`)
	_, err := e.Run(context.Background(), opts, nil)
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "IPERF3_PASSWORD=secret") {
		t.Errorf("err = %v, want it to contain the password env var", err)
	}
}

func TestParseOptionsPasswordValidation(t *testing.T) {
	if _, err := parseOptions(json.RawMessage(`{"host":"h","password":"x"}`)); err == nil {
		t.Error("want error for password without username/rsa_public_key_path")
	}
	if _, err := parseOptions(json.RawMessage(`{"host":"h","password":"x","username":"bob"}`)); err == nil {
		t.Error("want error for password without rsa_public_key_path")
	}
	if _, err := parseOptions(json.RawMessage(`{"host":"h","password":"x","username":"bob","rsa_public_key_path":"/k"}`)); err != nil {
		t.Errorf("valid password combo: %v", err)
	}
}

func TestRunFallsBackToSummaryWhenVersionProbeFails(t *testing.T) {
	// --version exits nonzero with unparseable output; the run must still
	// succeed using -J summary mode instead of aborting.
	body := `case "$1" in
  --version) echo "nope" >&2; exit 1;;
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
