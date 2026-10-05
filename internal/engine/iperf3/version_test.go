package iperf3

import (
	"context"
	"testing"

	"github.com/metril/speedtest-tracker/internal/engine/exectest"
)

func TestParseVersion(t *testing.T) {
	cases := map[string][2]int{
		"iperf 3.17.1 (cJSON 1.7.15)": {3, 17},
		"iperf 3.9\nLinux host":       {3, 9},
		"iperf 3.18":                  {3, 18},
	}
	for in, want := range cases {
		maj, min, err := parseVersion(in)
		if err != nil {
			t.Fatalf("parseVersion(%q): %v", in, err)
		}
		if maj != want[0] || min != want[1] {
			t.Errorf("parseVersion(%q) = %d.%d, want %d.%d", in, maj, min, want[0], want[1])
		}
	}
	if _, _, err := parseVersion("not a version"); err == nil {
		t.Error("want error")
	}
}

func TestSupportsJSONStream(t *testing.T) {
	newer := exectest.Build(t, "iperf3", `echo "iperf 3.17.1 (cJSON 1.7.15)"`)
	ok, err := supportsJSONStream(context.Background(), newer)
	if err != nil || !ok {
		t.Errorf("3.17.1: ok=%v err=%v, want true/nil", ok, err)
	}
	older := exectest.Build(t, "iperf3", `echo "iperf 3.12 (cJSON 1.7.15)"`)
	ok, err = supportsJSONStream(context.Background(), older)
	if err != nil || ok {
		t.Errorf("3.12: ok=%v err=%v, want false/nil", ok, err)
	}
	if _, err := supportsJSONStream(context.Background(), "/nonexistent/iperf3"); err == nil {
		t.Error("want error for missing binary")
	}
}

func TestSupportsConnectTimeout(t *testing.T) {
	for _, c := range []struct {
		maj, min int
		want     bool
	}{{3, 9, false}, {3, 10, true}, {3, 17, true}, {4, 0, true}, {2, 99, false}} {
		if got := supportsConnectTimeout(c.maj, c.min); got != c.want {
			t.Errorf("supportsConnectTimeout(%d.%d) = %v, want %v", c.maj, c.min, got, c.want)
		}
	}
}

func TestProbeCachesOnlyOnSuccess(t *testing.T) {
	bad := exectest.Build(t, "iperf3", `echo garbage`)
	e := &Engine{Bin: bad}
	if s, c := e.probe(); s || c || e.probed {
		t.Fatalf("failed probe = %v %v probed=%v, want false false and not cached", s, c, e.probed)
	}
	good := exectest.Build(t, "iperf3", `echo "iperf 3.17.1 (cJSON 1.7.15)"`)
	e.Bin = good
	if s, c := e.probe(); !s || !c || !e.probed {
		t.Fatalf("retry probe = %v %v probed=%v, want true true cached", s, c, e.probed)
	}
}
