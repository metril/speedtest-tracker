package iperf3

import (
	"encoding/json"
	"strings"
	"testing"
)

func args(t *testing.T, raw string, stream bool) string {
	t.Helper()
	o, err := parseOptions(json.RawMessage(raw))
	if err != nil {
		t.Fatalf("parseOptions(%s): %v", raw, err)
	}
	return strings.Join(buildArgs(o, stream), " ")
}

func TestBuildArgsTCPDefaults(t *testing.T) {
	got := args(t, `{"host":"nas.lan"}`, false)
	want := "-c nas.lan -p 5201 -J -t 10 -P 1"
	if got != want {
		t.Errorf("args = %q, want %q", got, want)
	}
}

func TestBuildArgsJSONStream(t *testing.T) {
	got := args(t, `{"host":"nas.lan"}`, true)
	if !strings.Contains(got, "--json-stream") || strings.Contains(got, " -J ") {
		t.Errorf("args = %q, want --json-stream instead of -J", got)
	}
}

func TestBuildArgsReverseBidirParallelBind(t *testing.T) {
	got := args(t, `{"host":"h","reverse":true,"parallel":4,"duration_s":30,"bind":"192.168.1.5","port":5202}`, false)
	for _, want := range []string{"-c h", "-p 5202", "-t 30", "-P 4", "-R", "-B 192.168.1.5"} {
		if !strings.Contains(got, want) {
			t.Errorf("args = %q, missing %q", got, want)
		}
	}
	got = args(t, `{"host":"h","bidir":true}`, false)
	if !strings.Contains(got, "--bidir") {
		t.Errorf("args = %q, missing --bidir", got)
	}
}

func TestBuildArgsUDP(t *testing.T) {
	got := args(t, `{"host":"h","protocol":"udp","udp_bitrate":"100M"}`, false)
	if !strings.Contains(got, "-u") || !strings.Contains(got, "-b 100M") {
		t.Errorf("args = %q", got)
	}
}

func TestBuildArgsAuth(t *testing.T) {
	got := args(t, `{"host":"h","username":"bob","rsa_public_key_path":"/etc/iperf/pub.pem"}`, false)
	if !strings.Contains(got, "--username bob") || !strings.Contains(got, "--rsa-public-key-path /etc/iperf/pub.pem") {
		t.Errorf("args = %q", got)
	}
}

func TestParseOptionsValidation(t *testing.T) {
	for _, raw := range []string{
		`{}`,
		`{"host":"h","port":0,"protocol":"sctp"}`,
		`{"host":"h","parallel":-1}`,
		`{"host":"h","duration_s":-5}`,
		`{"host":"h","reverse":true,"bidir":true}`,
		`{"host":"h",`,
		`{"host":"h","udp_bitrate":"100M"}`,              // udp_bitrate without protocol udp
		`{"host":"h","protocol":"udp","bidir":true}`,     // bidir with udp
		`{"host":"h","password":"x"}`,                    // password without username/rsa key
		`{"host":"h","password":"x","username":"bob"}`,   // password without rsa key
		`{"host":"h","port":5201,"port_range_end":5200}`, // port_range_end < port
		`{"host":"h","port_range_end":70000}`,            // port_range_end out of range
	} {
		if _, err := parseOptions(json.RawMessage(raw)); err == nil {
			t.Errorf("parseOptions(%s) = nil error, want error", raw)
		}
	}
}

func TestParseOptionsHostsNormalization(t *testing.T) {
	// A single-entry hosts list folds into Host, same as it being set
	// directly.
	o, err := parseOptions(json.RawMessage(`{"hosts":["a.lan"]}`))
	if err != nil {
		t.Fatalf("parseOptions: %v", err)
	}
	if o.Host != "a.lan" {
		t.Errorf("Host = %q, want a.lan", o.Host)
	}

	// A multi-entry hosts list is valid on its own (rotation picks one at
	// run time); Host is left empty.
	o, err = parseOptions(json.RawMessage(`{"hosts":["a.lan","b.lan"]}`))
	if err != nil {
		t.Fatalf("parseOptions with multi-host list: %v", err)
	}
	if o.Host != "" || len(o.Hosts) != 2 {
		t.Errorf("o = %+v, want empty Host and 2 Hosts", o)
	}

	// Host still wins when both are set.
	o, err = parseOptions(json.RawMessage(`{"host":"explicit","hosts":["a.lan"]}`))
	if err != nil {
		t.Fatalf("parseOptions: %v", err)
	}
	if o.Host != "explicit" {
		t.Errorf("Host = %q, want explicit", o.Host)
	}
}

func TestParseOptionsHostsValidation(t *testing.T) {
	for _, raw := range []string{
		`{}`,                       // no host, no hosts
		`{"hosts":[]}`,             // empty list, no host
		`{"hosts":["a.lan",""]}`,   // blank entry
	} {
		if _, err := parseOptions(json.RawMessage(raw)); err == nil {
			t.Errorf("parseOptions(%s) = nil error, want error", raw)
		}
	}
}

func TestParseOptionsPortRangeEndValid(t *testing.T) {
	o, err := parseOptions(json.RawMessage(`{"host":"h","port":5201,"port_range_end":5205}`))
	if err != nil {
		t.Fatalf("parseOptions: %v", err)
	}
	if o.PortRangeEnd != 5205 {
		t.Errorf("PortRangeEnd = %d, want 5205", o.PortRangeEnd)
	}
	// port_range_end == port is fine (a no-op range).
	if _, err := parseOptions(json.RawMessage(`{"host":"h","port":5201,"port_range_end":5201}`)); err != nil {
		t.Errorf("parseOptions with port_range_end==port: %v", err)
	}
}
