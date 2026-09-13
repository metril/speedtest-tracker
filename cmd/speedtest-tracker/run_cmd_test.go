package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/metril/speedtest-tracker/internal/settings"
)

// withTestDB points ST_DB_PATH at a fresh temp file so runCmd's settings
// lookup never touches a real database.
func withTestDB(t *testing.T) {
	t.Helper()
	t.Setenv("ST_DB_PATH", filepath.Join(t.TempDir(), "test.db"))
}

func TestBuildRegistryHasAllEngines(t *testing.T) {
	r := buildRegistry(settings.Engines{SpeedtestBin: "speedtest", Iperf3Bin: "iperf3"})
	got := strings.Join(r.Names(), ",")
	if got != "cloudflare,fake,iperf3,ookla" {
		t.Errorf("Names() = %q", got)
	}
}

func TestRunCmdFakeEngine(t *testing.T) {
	withTestDB(t)
	var out, errBuf bytes.Buffer
	code := runCmd(context.Background(), []string{"--engine", "fake", "--opts", "{}"}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, errBuf.String())
	}
	var res struct {
		DownloadBps float64 `json:"download_bps"`
		ServerName  string  `json:"server_name"`
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, out.String())
	}
	if res.DownloadBps != 100_000_000 || res.ServerName != "fake-server" {
		t.Errorf("result = %+v", res)
	}
	if !strings.Contains(errBuf.String(), `"phase":"done"`) {
		t.Errorf("stderr lacks progress lines:\n%s", errBuf.String())
	}
}

func TestRunCmdUnknownEngine(t *testing.T) {
	withTestDB(t)
	var out, errBuf bytes.Buffer
	if code := runCmd(context.Background(), []string{"--engine", "nope"}, &out, &errBuf); code == 0 {
		t.Fatal("want non-zero exit")
	}
	if !strings.Contains(errBuf.String(), "unknown engine") {
		t.Errorf("stderr = %s", errBuf.String())
	}
}

func TestRunCmdEngineFailure(t *testing.T) {
	withTestDB(t)
	var out, errBuf bytes.Buffer
	code := runCmd(context.Background(), []string{"--engine", "fake", "--opts", `{"fail":true}`}, &out, &errBuf)
	if code == 0 {
		t.Fatal("want non-zero exit")
	}
	if !strings.Contains(errBuf.String(), "forced failure") {
		t.Errorf("stderr = %s", errBuf.String())
	}
}

func TestRunCmdInvalidOpts(t *testing.T) {
	withTestDB(t)
	var out, errBuf bytes.Buffer
	if code := runCmd(context.Background(), []string{"--engine", "iperf3", "--opts", "{}"}, &out, &errBuf); code == 0 {
		t.Fatal("want non-zero exit for missing host")
	}
}

func TestRunCmdBinOverrides(t *testing.T) {
	withTestDB(t)
	var out, errBuf bytes.Buffer
	// The fake engine ignores bin overrides, but this exercises the flag
	// parsing and settings-merge path end to end without a real binary.
	code := runCmd(context.Background(), []string{
		"--engine", "fake", "--opts", "{}",
		"--speedtest-bin", "/custom/speedtest",
		"--iperf3-bin", "/custom/iperf3",
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, errBuf.String())
	}
}
