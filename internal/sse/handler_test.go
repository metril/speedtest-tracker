package sse

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandlerStreamsEvents(t *testing.T) {
	h := NewHub()
	srv := httptest.NewServer(Handler(h))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("content type = %q", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "no-cache") {
		t.Errorf("cache control = %q", cc)
	}

	// Wait for the handler to register before publishing.
	deadline := time.Now().Add(2 * time.Second)
	for h.Subscribers() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	h.Publish(Event{Type: EventRun, Data: []byte(`{"run_id":1}`)})

	sc := bufio.NewScanner(resp.Body)
	var gotEvent, gotData bool
	for sc.Scan() {
		line := sc.Text()
		if line == "event: run" {
			gotEvent = true
		}
		if line == `data: {"run_id":1}` {
			gotData = true
			break
		}
	}
	if !gotEvent || !gotData {
		t.Errorf("event=%v data=%v", gotEvent, gotData)
	}
}

func TestHandlerUnsubscribesOnClientDisconnect(t *testing.T) {
	h := NewHub()
	srv := httptest.NewServer(Handler(h))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for h.Subscribers() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	resp.Body.Close()

	deadline = time.Now().Add(2 * time.Second)
	for h.Subscribers() != 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := h.Subscribers(); got != 0 {
		t.Errorf("Subscribers = %d after disconnect, want 0", got)
	}
}
