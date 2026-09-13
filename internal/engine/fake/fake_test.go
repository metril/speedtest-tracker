package fake

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/metril/speedtest-tracker/internal/engine"
)

func TestRunEmitsDeterministicProgressAndResult(t *testing.T) {
	e := New()
	var got []engine.Progress
	res, err := e.Run(context.Background(), json.RawMessage(`{}`), func(p engine.Progress) {
		got = append(got, p)
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.DownloadBps != 100_000_000 || res.UploadBps != 50_000_000 {
		t.Errorf("result = %+v", res)
	}
	if res.PingMs != 12.5 || res.ServerName != "fake-server" {
		t.Errorf("result = %+v", res)
	}
	if len(got) == 0 {
		t.Fatal("no progress events")
	}
	if got[0].Phase != engine.PhaseConnecting {
		t.Errorf("first phase = %q, want connecting", got[0].Phase)
	}
	if last := got[len(got)-1]; last.Phase != engine.PhaseDone || last.Progress != 1 {
		t.Errorf("last event = %+v", last)
	}
	// Second run must produce an identical event stream.
	var again []engine.Progress
	if _, err := New().Run(context.Background(), json.RawMessage(`{}`), func(p engine.Progress) {
		again = append(again, p)
	}); err != nil {
		t.Fatal(err)
	}
	if len(again) != len(got) {
		t.Fatalf("event count %d != %d", len(again), len(got))
	}
	for i := range got {
		if got[i] != again[i] {
			t.Fatalf("event %d differs: %+v vs %+v", i, got[i], again[i])
		}
	}
}

func TestRunFailOption(t *testing.T) {
	var last engine.Progress
	_, err := New().Run(context.Background(), json.RawMessage(`{"fail":true}`), func(p engine.Progress) { last = p })
	if err == nil {
		t.Fatal("want error")
	}
	if last.Phase != engine.PhaseError {
		t.Errorf("last phase = %q, want error", last.Phase)
	}
}

func TestValidateRejectsBadJSON(t *testing.T) {
	if err := New().Validate(json.RawMessage(`{"fail":`)); err == nil {
		t.Fatal("want error")
	}
	if err := New().Validate(nil); err != nil {
		t.Fatalf("nil opts: %v", err)
	}
}

func TestRunHonoursContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := New().Run(ctx, nil, nil); err == nil {
		t.Fatal("want context error")
	}
}
