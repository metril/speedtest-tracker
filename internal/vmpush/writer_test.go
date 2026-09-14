package vmpush_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/metril/speedtest-tracker/internal/store"
	"github.com/metril/speedtest-tracker/internal/vmpush"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func okResult() *store.Result {
	return &store.Result{
		Status: "ok", StartedAt: "2026-09-14T10:00:00.000Z",
		DownloadBps: 1e9, UploadBps: 5e8, DurationMs: 100,
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met within 3s")
}

func TestWriterPostsBatchToImportEndpoint(t *testing.T) {
	type got struct {
		path, auth, body string
	}
	ch := make(chan got, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		ch <- got{r.URL.Path, r.Header.Get("Authorization"), string(b)}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	wr := vmpush.New(vmpush.Config{Logger: discardLogger()})
	wr.Configure(true, srv.URL, "Bearer tok", map[string]string{"host": "pi4"})
	wr.Start()
	defer wr.Close(context.Background())

	wr.OnResult(context.Background(), okResult(), vmpush.Meta{Schedule: "nightly"})
	select {
	case g := <-ch:
		if g.path != "/api/v1/import/prometheus" {
			t.Fatalf("path = %s", g.path)
		}
		if g.auth != "Bearer tok" {
			t.Fatalf("auth header = %q", g.auth)
		}
		if !strings.Contains(g.body, `host="pi4"`) {
			t.Fatalf("extra label missing: %s", g.body)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no push within 2s")
	}
	waitFor(t, func() bool { return wr.Stats().Pushed == 1 })
}

func TestWriterDisabledDoesNotPush(t *testing.T) {
	hits := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits <- struct{}{} }))
	defer srv.Close()
	wr := vmpush.New(vmpush.Config{Logger: discardLogger()})
	wr.Configure(false, srv.URL, "", nil)
	wr.Start()
	defer wr.Close(context.Background())
	wr.OnResult(context.Background(), okResult(), vmpush.Meta{})
	select {
	case <-hits:
		t.Fatal("pushed while disabled")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestWriterRetriesAfterFailureAndCountsFailures(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	wr := vmpush.New(vmpush.Config{Logger: discardLogger(), MaxBackoff: 10 * time.Millisecond})
	wr.Configure(true, srv.URL, "", nil)
	wr.Start()
	defer wr.Close(context.Background())
	wr.OnResult(context.Background(), okResult(), vmpush.Meta{})
	waitFor(t, func() bool { s := wr.Stats(); return s.Pushed == 1 && s.Failed >= 1 })
}

func TestWriterRingDropsOldestWhenFull(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	defer close(block)
	wr := vmpush.New(vmpush.Config{Logger: discardLogger(), RingSize: 2, MaxBackoff: time.Millisecond})
	wr.Configure(true, srv.URL, "", nil)
	wr.Start()
	defer wr.Close(context.Background())
	for i := 0; i < 50; i++ {
		wr.OnResult(context.Background(), okResult(), vmpush.Meta{})
	}
	waitFor(t, func() bool { return wr.Stats().Dropped > 0 })
}

func TestOnResultNeverBlocks(t *testing.T) {
	wr := vmpush.New(vmpush.Config{Logger: discardLogger(), RingSize: 1})
	wr.Configure(true, "http://127.0.0.1:1", "", nil) // nothing listening
	wr.Start()
	defer wr.Close(context.Background())
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			wr.OnResult(context.Background(), okResult(), vmpush.Meta{})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("OnResult blocked the caller")
	}
}
