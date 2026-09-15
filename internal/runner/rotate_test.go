package runner

import (
	"encoding/json"
	"testing"
)

func TestRotateOoklaPicksIndex(t *testing.T) {
	got, ok := rotate(json.RawMessage(`{"server_ids":[111,222,333]}`), "ookla", func(n int) int {
		if n != 3 {
			t.Errorf("n = %d, want 3", n)
		}
		return 1
	})
	if !ok {
		t.Fatal("rotate returned ok=false")
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(got, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if string(m["server_id"]) != "222" {
		t.Errorf("server_id = %s, want 222", m["server_id"])
	}
	if _, present := m["server_ids"]; present {
		t.Error("server_ids must be dropped from the rewritten options")
	}
}

func TestRotateIperf3PicksIndex(t *testing.T) {
	got, ok := rotate(json.RawMessage(`{"hosts":["a","b","c"],"port":5202}`), "iperf3", func(int) int { return 2 })
	if !ok {
		t.Fatal("rotate returned ok=false")
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(got, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if string(m["host"]) != `"c"` {
		t.Errorf("host = %s, want \"c\"", m["host"])
	}
	if string(m["port"]) != "5202" {
		t.Errorf("port = %s, want 5202 (untouched)", m["port"])
	}
	if _, present := m["hosts"]; present {
		t.Error("hosts must be dropped from the rewritten options")
	}
}

func TestRotateNoopCases(t *testing.T) {
	pickCalled := false
	pick := func(int) int { pickCalled = true; return 0 }

	cases := []struct {
		name   string
		opts   string
		engine string
	}{
		{"cloudflare engine", `{"server_ids":[1,2]}`, "cloudflare"},
		{"unknown engine", `{"hosts":["a","b"]}`, "fake"},
		{"no list key", `{"server_id":1}`, "ookla"},
		{"empty list", `{"server_ids":[]}`, "ookla"},
		{"single entry list", `{"server_ids":[111]}`, "ookla"},
		{"single entry hosts", `{"hosts":["a"]}`, "iperf3"},
		{"unparseable options", `not json`, "ookla"},
	}
	for _, c := range cases {
		pickCalled = false
		got, ok := rotate(json.RawMessage(c.opts), c.engine, pick)
		if ok {
			t.Errorf("%s: ok = true, want false", c.name)
		}
		if string(got) != c.opts {
			t.Errorf("%s: opts = %s, want unchanged %s", c.name, got, c.opts)
		}
		if pickCalled {
			t.Errorf("%s: pick was called, want it skipped", c.name)
		}
	}
}

func TestRotateOutOfRangePickFallsBackToZero(t *testing.T) {
	got, ok := rotate(json.RawMessage(`{"server_ids":[111,222]}`), "ookla", func(int) int { return 99 })
	if !ok {
		t.Fatal("rotate returned ok=false")
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(got, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if string(m["server_id"]) != "111" {
		t.Errorf("server_id = %s, want 111 (fallback to index 0)", m["server_id"])
	}
}
