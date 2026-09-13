package engine

import (
	"context"
	"encoding/json"
	"testing"
)

type stubEngine struct{ name string }

func (s stubEngine) Name() string                   { return s.name }
func (s stubEngine) Validate(json.RawMessage) error { return nil }
func (s stubEngine) Run(context.Context, json.RawMessage, func(Progress)) (*Result, error) {
	return &Result{DownloadBps: 1}, nil
}

func TestRegistryGetAndNames(t *testing.T) {
	r := NewRegistry()
	r.Register(stubEngine{name: "zeta"})
	r.Register(stubEngine{name: "alpha"})

	if _, ok := r.Get("missing"); ok {
		t.Error("Get(missing) = ok, want not ok")
	}
	e, ok := r.Get("alpha")
	if !ok || e.Name() != "alpha" {
		t.Fatalf("Get(alpha) = %v, %v", e, ok)
	}
	got := r.Names()
	if len(got) != 2 || got[0] != "alpha" || got[1] != "zeta" {
		t.Errorf("Names() = %v, want [alpha zeta]", got)
	}
}

func TestEmitNilSafe(t *testing.T) {
	Emit(nil, Progress{Phase: PhasePing}) // must not panic

	var got []Progress
	Emit(func(p Progress) { got = append(got, p) }, Progress{Phase: PhaseDownload, Progress: 0.5})
	if len(got) != 1 || got[0].Phase != PhaseDownload || got[0].Progress != 0.5 {
		t.Errorf("got %+v", got)
	}
}

func TestProgressJSONFieldNames(t *testing.T) {
	b, err := json.Marshal(Progress{Phase: PhaseUpload, Progress: 0.25, Bps: 1e6,
		PingMs: 12, JitterMs: 1, LossPct: 0, ElapsedMs: 1500, ServerName: "srv"})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"phase":"upload","progress":0.25,"bps":1000000,"ping_ms":12,"jitter_ms":1,"loss_pct":0,"elapsed_ms":1500,"server_name":"srv"}`
	if string(b) != want {
		t.Errorf("json = %s\nwant %s", b, want)
	}
}
