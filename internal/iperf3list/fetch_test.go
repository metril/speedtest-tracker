package iperf3list

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetchParsesWellFormedRows(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[
			{"IP/HOST":"iperf.example.net","PORT":"5201","OPTIONS":"-R,-u,-6","GB/S":"1","CONTINENT":"EU","COUNTRY":"DE","SITE":"Frankfurt","PROVIDER":"Example Net"},
			{"IP/HOST":"range.example.net","PORT":"9205-9240","OPTIONS":"","GB/S":"10","CONTINENT":"NA","COUNTRY":"US","SITE":"Denver","PROVIDER":"Other Net"}
		]`))
	}))
	defer srv.Close()

	got, err := Fetch(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d servers, want 2: %+v", len(got), got)
	}
	if got[0].Host != "iperf.example.net" || got[0].Port != 5201 || got[0].PortEnd != 0 ||
		!got[0].SupportsReverse || !got[0].SupportsUDP || !got[0].SupportsIPv6 {
		t.Errorf("row 0 = %+v", got[0])
	}
	if got[0].Site != "Frankfurt" || got[0].Country != "DE" || got[0].Provider != "Example Net" || got[0].Continent != "EU" || got[0].GBs != "1" {
		t.Errorf("row 0 metadata = %+v", got[0])
	}
	// A port range keeps both the start and the end port.
	if got[1].Host != "range.example.net" || got[1].Port != 9205 || got[1].PortEnd != 9240 ||
		got[1].SupportsReverse || got[1].SupportsUDP || got[1].SupportsIPv6 {
		t.Errorf("row 1 = %+v", got[1])
	}
}

func TestFetchSkipsMalformedRowsWithoutFailingTheBatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[
			{"IP/HOST":"","PORT":"5201"},
			{"IP/HOST":"noport.example.net","PORT":""},
			{"IP/HOST":"badport.example.net","PORT":"abc"},
			{"IP/HOST":"  ","PORT":"5201"},
			{"IP/HOST":"good.example.net","PORT":"5201","OPTIONS":"-R"}
		]`))
	}))
	defer srv.Close()

	got, err := Fetch(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Host != "good.example.net" {
		t.Fatalf("got = %+v, want only the one well-formed row", got)
	}
}

func TestFetchReturnsEmptySliceOnEmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	got, err := Fetch(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d servers, want 0", len(got))
	}
}

func TestFetchErrorsOnHTTPFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	if _, err := Fetch(context.Background(), srv.Client(), srv.URL); err == nil {
		t.Fatal("want an error on a non-200 response")
	}
}

func TestFetchErrorsOnMalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	if _, err := Fetch(context.Background(), srv.Client(), srv.URL); err == nil {
		t.Fatal("want an error on malformed JSON")
	}
}

func TestFetchRespectsContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Fetch(ctx, srv.Client(), srv.URL); err == nil {
		t.Fatal("want an error when the context is already canceled")
	}
}

func TestParsePortRange(t *testing.T) {
	cases := []struct {
		in             string
		wantStart, end int
		ok             bool
	}{
		{"5201", 5201, 0, true},
		{"9205-9240", 9205, 9240, true},
		{"", 0, 0, false},
		{"abc", 0, 0, false},
		{"0", 0, 0, false},
		{"-5", 0, 0, false},
		{" 5201 ", 5201, 0, true},
		{"5201-abc", 5201, 0, true}, // malformed end still leaves the start usable
	}
	for _, c := range cases {
		start, end, ok := parsePortRange(c.in)
		if start != c.wantStart || end != c.end || ok != c.ok {
			t.Errorf("parsePortRange(%q) = %d,%d,%v want %d,%d,%v", c.in, start, end, ok, c.wantStart, c.end, c.ok)
		}
	}
}

func TestParseOptions(t *testing.T) {
	cases := []struct {
		in                 string
		reverse, udp, ipv6 bool
	}{
		{"-R,-u", true, true, false},
		{"-R", true, false, false},
		{"-u", false, true, false},
		{"-6", false, false, true},
		{"-R,-u,-6", true, true, true},
		{"", false, false, false},
		{"-Z", false, false, false},
		{" -R , -u , -6 ", true, true, true},
	}
	for _, c := range cases {
		reverse, udp, ipv6 := parseOptions(c.in)
		if reverse != c.reverse || udp != c.udp || ipv6 != c.ipv6 {
			t.Errorf("parseOptions(%q) = %v,%v,%v want %v,%v,%v", c.in, reverse, udp, ipv6, c.reverse, c.udp, c.ipv6)
		}
	}
}

func TestFetchRejectsOversizedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[`))
		chunk := bytes.Repeat([]byte(`{"IP/HOST":"x.example.net","PORT":"5201"},`), 4096)
		for i := 0; i < (maxResponseBytes/len(chunk))+2; i++ {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	// A truncated body must fail cleanly, not succeed on a partial parse.
	if _, err := Fetch(context.Background(), srv.Client(), srv.URL); err == nil {
		t.Fatal("Fetch: want error for oversized response, got nil")
	}
}

func TestFetchRefusesCrossHostRedirect(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[]`))
	}))
	defer target.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.RequestURI(), http.StatusFound)
	}))
	defer redirector.Close()

	client := &http.Client{CheckRedirect: rejectCrossHostRedirect}
	if _, err := Fetch(context.Background(), client, redirector.URL); err == nil {
		t.Fatal("Fetch: want error for cross-host redirect, got nil")
	}
}

func TestFetchUsesGivenClientAndURL(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	if _, err := Fetch(context.Background(), srv.Client(), srv.URL+"/listed_iperf3_servers.json"); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(gotPath, "/listed_iperf3_servers.json") {
		t.Errorf("gotPath = %q", gotPath)
	}
}
