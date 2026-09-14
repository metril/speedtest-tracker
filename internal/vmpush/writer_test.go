package vmpush_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

// TestDeliverAbandonsBatchWhenDisabledMidRetry is the regression case for
// the finding that deliver retried forever without re-checking enabled, so
// disabling an integration never stopped an in-flight retry loop.
func TestDeliverAbandonsBatchWhenDisabledMidRetry(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	wr := vmpush.New(vmpush.Config{Logger: discardLogger(), MaxBackoff: 5 * time.Millisecond})
	wr.Configure(true, srv.URL, "", nil)
	wr.Start()
	defer wr.Close(context.Background())

	wr.OnResult(context.Background(), okResult(), vmpush.Meta{})
	waitFor(t, func() bool { return calls.Load() >= 2 }) // confirm it is actually retrying

	wr.Configure(false, "", "", nil)
	waitFor(t, func() bool { return wr.Stats().Failed >= 1 })

	seen := calls.Load()
	time.Sleep(50 * time.Millisecond) // several more backoff cycles, if it kept retrying
	if calls.Load() != seen {
		t.Fatalf("kept retrying after being disabled: calls %d -> %d", seen, calls.Load())
	}
}

// TestOnResultConcurrentEnqueueAccountsForEveryCall is the regression case
// for the finding that the drop-oldest dance in OnResult was three
// non-atomic selects shared across concurrent callers, which could
// double-drop or steal a batch. Under concurrent producers, every call must
// land in exactly one bucket: pushed, dropped, or failed.
func TestOnResultConcurrentEnqueueAccountsForEveryCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	wr := vmpush.New(vmpush.Config{Logger: discardLogger(), RingSize: 4})
	wr.Configure(true, srv.URL, "", nil)
	wr.Start()

	const n = 500
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			wr.OnResult(context.Background(), okResult(), vmpush.Meta{})
		}()
	}
	wg.Wait()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := wr.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}

	s := wr.Stats()
	if total := s.Pushed + s.Dropped + s.Failed; total != n {
		t.Fatalf("accounted for %d of %d calls (pushed=%d dropped=%d failed=%d)",
			total, n, s.Pushed, s.Dropped, s.Failed)
	}
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

// TestCloseDrainsQueuedBatches is the regression case for the finding that
// run() returned on <-w.stop without draining w.in, discarding every queued
// batch at shutdown despite Close's "waits to drain" doc. The server blocks
// the first request so the other two batches are still sitting in the ring
// when Close is called, deterministically forcing the shutdown drain path.
func TestCloseDrainsQueuedBatches(t *testing.T) {
	block := make(chan struct{})
	started := make(chan struct{}, 1)
	hits := make(chan struct{}, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-block
		hits <- struct{}{}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	wr := vmpush.New(vmpush.Config{Logger: discardLogger(), RingSize: 10})
	wr.Configure(true, srv.URL, "", nil)
	wr.Start()

	for i := 0; i < 3; i++ {
		wr.OnResult(context.Background(), okResult(), vmpush.Meta{})
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("first batch never reached the server")
	}
	// Batches 2 and 3 are now queued in the ring while the worker blocks
	// delivering batch 1.

	closeErr := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		closeErr <- wr.Close(ctx)
	}()
	close(block) // let batch 1 complete, and 2/3 (drained on shutdown) proceed

	if err := <-closeErr; err != nil {
		t.Fatalf("Close: %v", err)
	}
	waitFor(t, func() bool { return len(hits) == 3 })
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
