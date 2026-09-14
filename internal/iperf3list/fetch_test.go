package iperf3list

import (
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
			{"IP/HOST":"iperf.example.net","PORT":"5201","OPTIONS":"-R,-u","GB/S":"1","CONTINENT":"EU","COUNTRY":"DE","SITE":"Frankfurt","PROVIDER":"Example Net"},
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
	if got[0].Host != "iperf.example.net" || got[0].Port != 5201 || !got[0].SupportsReverse || !got[0].SupportsUDP {
		t.Errorf("row 0 = %+v", got[0])
	}
	if got[0].Site != "Frankfurt" || got[0].Country != "DE" || got[0].Provider != "Example Net" || got[0].Continent != "EU" || got[0].GBs != "1" {
		t.Errorf("row 0 metadata = %+v", got[0])
	}
	// A port range takes its first port.
	if got[1].Host != "range.example.net" || got[1].Port != 9205 || got[1].SupportsReverse || got[1].SupportsUDP {
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

func TestParsePort(t *testing.T) {
	cases := []struct {
		in   string
		want int
		ok   bool
	}{
		{"5201", 5201, true},
		{"9205-9240", 9205, true},
		{"", 0, false},
		{"abc", 0, false},
		{"0", 0, false},
		{"-5", 0, false},
		{" 5201 ", 5201, true},
	}
	for _, c := range cases {
		got, ok := parsePort(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("parsePort(%q) = %d,%v want %d,%v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestParseOptions(t *testing.T) {
	cases := []struct {
		in           string
		reverse, udp bool
	}{
		{"-R,-u", true, true},
		{"-R", true, false},
		{"-u", false, true},
		{"", false, false},
		{"-Z", false, false},
		{" -R , -u ", true, true},
	}
	for _, c := range cases {
		reverse, udp := parseOptions(c.in)
		if reverse != c.reverse || udp != c.udp {
			t.Errorf("parseOptions(%q) = %v,%v want %v,%v", c.in, reverse, udp, c.reverse, c.udp)
		}
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
