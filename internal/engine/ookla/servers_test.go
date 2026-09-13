package ookla

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/metril/speedtest-tracker/internal/engine/exectest"
)

// countingStub emits the servers fixture and appends a line to a counter
// file on every invocation, so the test can assert cache hits.
func countingStub(t *testing.T, counter string) string {
	t.Helper()
	body := "echo x >> " + counter + "\n" + exectest.ScriptEmitFile(fixturePath(t, "servers.json"), 0)
	return exectest.Build(t, "speedtest", body)
}

func invocations(t *testing.T, counter string) int {
	t.Helper()
	b, err := os.ReadFile(counter)
	if err != nil {
		return 0
	}
	return bytes.Count(b, []byte("\n"))
}

func TestServersParsesAndCaches(t *testing.T) {
	counter := filepath.Join(t.TempDir(), "count")
	l := NewServerList(countingStub(t, counter), time.Hour)

	now := time.Unix(0, 0)
	l.SetNow(func() time.Time { return now })

	got, err := l.Servers(context.Background())
	if err != nil {
		t.Fatalf("Servers: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d servers, want 2", len(got))
	}
	if got[0].ID != "12345" || got[0].Name != "Example Server" ||
		got[0].Location != "Berlin" || got[0].Country != "Germany" ||
		got[0].Host != "speed.example.net:8080" {
		t.Errorf("servers[0] = %+v", got[0])
	}

	if _, err := l.Servers(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := invocations(t, counter); n != 1 {
		t.Errorf("binary invoked %d times, want 1 (cache hit expected)", n)
	}

	now = now.Add(2 * time.Hour)
	if _, err := l.Servers(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := invocations(t, counter); n != 2 {
		t.Errorf("binary invoked %d times after TTL expiry, want 2", n)
	}
}

func TestServersInvalidate(t *testing.T) {
	counter := filepath.Join(t.TempDir(), "count")
	l := NewServerList(countingStub(t, counter), time.Hour)
	if _, err := l.Servers(context.Background()); err != nil {
		t.Fatal(err)
	}
	l.Invalidate()
	if _, err := l.Servers(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := invocations(t, counter); n != 2 {
		t.Errorf("binary invoked %d times, want 2", n)
	}
}

func TestServersErrorNotCached(t *testing.T) {
	bin := exectest.Build(t, "speedtest", "exit 2")
	l := NewServerList(bin, time.Hour)
	if _, err := l.Servers(context.Background()); err == nil {
		t.Fatal("want error")
	}
}
