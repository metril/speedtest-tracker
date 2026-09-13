package ookla

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

func fixturePath(t *testing.T, name string) string {
	t.Helper()
	p, err := filepath.Abs("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestArgs(t *testing.T) {
	e := New("speedtest", Config{AcceptLicense: true, AcceptGDPR: true})
	got := strings.Join(e.Args(Options{}), " ")
	want := "-f jsonl --progress=yes --accept-license --accept-gdpr"
	if got != want {
		t.Errorf("Args = %q, want %q", got, want)
	}
	got = strings.Join(e.Args(Options{ServerID: 12345}), " ")
	if !strings.Contains(got, "-s 12345") {
		t.Errorf("Args with server = %q", got)
	}
	bare := New("speedtest", Config{})
	if strings.Contains(strings.Join(bare.Args(Options{}), " "), "--accept-license") {
		t.Error("consent flags must be omitted when disabled")
	}
}

func TestRunSuccess(t *testing.T) {
	bin := exectest.Build(t, "speedtest", exectest.ScriptEmitFile(fixturePath(t, "tcp_success.jsonl"), 0))
	e := New(bin, Config{AcceptLicense: true, AcceptGDPR: true})

	var events []engine.Progress
	res, err := e.Run(context.Background(), json.RawMessage(`{"server_id":12345}`), func(p engine.Progress) {
		events = append(events, p)
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.DownloadBps != 92_000_000 || res.ServerID != "12345" {
		t.Errorf("result = %+v", res)
	}
	if len(events) == 0 {
		t.Error("no progress events")
	}
}

func TestRunErrorExit(t *testing.T) {
	bin := exectest.Build(t, "speedtest", exectest.ScriptEmitFile(fixturePath(t, "error_dns.jsonl"), 1))
	_, err := New(bin, Config{}).Run(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "name resolution") {
		t.Errorf("err = %v, want the CLI error message", err)
	}
}

func TestRunSuccessDespiteNonZeroExit(t *testing.T) {
	// A parsed result line must win over a nonzero exit code: some CLI
	// versions exit nonzero after already printing a valid result record.
	bin := exectest.Build(t, "speedtest", exectest.ScriptEmitFile(fixturePath(t, "tcp_success.jsonl"), 1))
	res, err := New(bin, Config{}).Run(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.DownloadBps != 92_000_000 {
		t.Errorf("result = %+v", res)
	}
}

func TestRunErrorExitWithStderr(t *testing.T) {
	// No stdout and no JSON error line: the error must surface stderr.
	bin := exectest.Build(t, "speedtest", "echo boom >&2\nexit 2")
	_, err := New(bin, Config{}).Run(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("err = %v, want it to contain stderr output", err)
	}
}

func TestRunHonoursContextCancel(t *testing.T) {
	bin := exectest.Build(t, "speedtest", "sleep 30")
	e := New(bin, Config{})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	done := make(chan error, 1)
	go func() {
		_, err := e.Run(ctx, nil, nil)
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

func TestRunMissingBinary(t *testing.T) {
	_, err := New("/nonexistent/speedtest", Config{}).Run(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("want error")
	}
}

func TestValidate(t *testing.T) {
	e := New("speedtest", Config{})
	if err := e.Validate(json.RawMessage(`{"server_id":1}`)); err != nil {
		t.Errorf("valid opts: %v", err)
	}
	if err := e.Validate(json.RawMessage(`{"server_id":-1}`)); err == nil {
		t.Error("want error for negative server_id")
	}
	if err := e.Validate(json.RawMessage(`{`)); err == nil {
		t.Error("want error for bad json")
	}
}

func TestName(t *testing.T) {
	if New("speedtest", Config{}).Name() != "ookla" {
		t.Error("Name mismatch")
	}
}
