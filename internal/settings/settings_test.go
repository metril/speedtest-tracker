package settings

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/metril/speedtest-tracker/internal/store"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	s, err := New(context.Background(), st)
	if err != nil {
		t.Fatalf("settings.New: %v", err)
	}
	return s
}

func TestNewSeedsGeneralDefaults(t *testing.T) {
	s := newTestStore(t)
	g, err := s.General(context.Background())
	if err != nil {
		t.Fatalf("General: %v", err)
	}
	if g.Timezone != "UTC" {
		t.Errorf("Timezone = %q, want UTC", g.Timezone)
	}
	if g.Units != "Mbps" {
		t.Errorf("Units = %q, want Mbps", g.Units)
	}
	if g.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want info", g.LogLevel)
	}
	if g.RetentionDaysResults != 90 {
		t.Errorf("RetentionDaysResults = %d, want 90", g.RetentionDaysResults)
	}
	if g.RetentionDaysRuns != 30 {
		t.Errorf("RetentionDaysRuns = %d, want 30", g.RetentionDaysRuns)
	}
}

func TestSetGetRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	if err := s.Set(ctx, "general.timezone", "Europe/London"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	raw, ok, err := s.Get(ctx, "general.timezone")
	if err != nil || !ok {
		t.Fatalf("Get: raw=%s ok=%v err=%v", raw, ok, err)
	}
	if string(raw) != `"Europe/London"` {
		t.Errorf("raw = %s, want \"Europe/London\"", raw)
	}
	g, err := s.General(ctx)
	if err != nil {
		t.Fatalf("General: %v", err)
	}
	if g.Timezone != "Europe/London" {
		t.Errorf("Timezone = %q", g.Timezone)
	}
}

func TestGetMissingKey(t *testing.T) {
	_, ok, err := newTestStore(t).Get(context.Background(), "general.nope")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if ok {
		t.Error("ok = true for missing key, want false")
	}
}

func TestSubscribeReceivesChangedKey(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	ch, cancel := s.Subscribe()
	defer cancel()

	if err := s.Set(ctx, "general.log_level", "debug"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	select {
	case key := <-ch:
		if key != "general.log_level" {
			t.Errorf("key = %q, want general.log_level", key)
		}
	case <-time.After(time.Second):
		t.Fatal("no notification within 1s")
	}
}

func TestSubscribeDoesNotBlockOnSlowSubscriber(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	_, cancel := s.Subscribe() // never drained
	defer cancel()

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			s.Set(ctx, "general.base_url", "http://x")
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Set blocked on a slow subscriber")
	}
}
