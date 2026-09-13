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
