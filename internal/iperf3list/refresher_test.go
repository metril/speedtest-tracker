package iperf3list

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/metril/speedtest-tracker/internal/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "iperf3list.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

const oneServerFixture = `[{"IP/HOST":"iperf.example.net","PORT":"5201","OPTIONS":"-R"}]`

func countingServer(t *testing.T, body string) (*httptest.Server, *int32) {
	t.Helper()
	var count int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&count, 1)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &count
}

func TestRefreshNowReplacesAndReportsCount(t *testing.T) {
	db := newTestStore(t)
	srv, _ := countingServer(t, oneServerFixture)

	rf := New(Config{Store: db, URL: srv.URL})
	fetchedAt, count, err := rf.RefreshNow(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
	if fetchedAt.IsZero() {
		t.Fatal("fetchedAt is zero, want a stamped time")
	}
	n, _ := db.CountIperf3Servers(context.Background())
	if n != 1 {
		t.Fatalf("store has %d rows, want 1", n)
	}
}

func TestRefreshNowKeepsExistingTableOnEmptyFetch(t *testing.T) {
	db := newTestStore(t)
	if err := db.ReplaceIperf3Servers(context.Background(), []store.Iperf3Server{{Host: "existing.example.net", Port: 5201}}); err != nil {
		t.Fatal(err)
	}
	before, _ := db.Iperf3ServersFetchedAt(context.Background())

	srv, _ := countingServer(t, `[]`)
	rf := New(Config{Store: db, URL: srv.URL})
	fetchedAt, count, err := rf.RefreshNow(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1 (existing row kept)", count)
	}
	after, _ := db.Iperf3ServersFetchedAt(context.Background())
	if after != before {
		t.Fatalf("fetched_at changed on an empty fetch: before=%q after=%q", before, after)
	}
	if fetchedAt.IsZero() {
		t.Fatal("fetchedAt is zero, want the existing stamped time")
	}
}

func TestRefreshNowReturnsErrorOnFetchFailure(t *testing.T) {
	db := newTestStore(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	rf := New(Config{Store: db, URL: srv.URL})
	if _, _, err := rf.RefreshNow(context.Background()); err == nil {
		t.Fatal("want an error when the upstream fetch fails")
	}
}

func TestRunRefreshesOnStartWhenTableIsEmpty(t *testing.T) {
	db := newTestStore(t)
	srv, calls := countingServer(t, oneServerFixture)

	rf := New(Config{Store: db, URL: srv.URL, Interval: time.Hour})
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	rf.Run(ctx)

	if atomic.LoadInt32(calls) < 1 {
		t.Fatal("Run did not refresh on start with an empty table")
	}
	n, _ := db.CountIperf3Servers(context.Background())
	if n != 1 {
		t.Fatalf("store has %d rows, want 1", n)
	}
}

func TestRunSkipsStartupRefreshWhenRecentlyFetched(t *testing.T) {
	db := newTestStore(t)
	if err := db.ReplaceIperf3Servers(context.Background(), []store.Iperf3Server{{Host: "existing.example.net", Port: 5201}}); err != nil {
		t.Fatal(err)
	}
	srv, calls := countingServer(t, oneServerFixture)

	rf := New(Config{Store: db, URL: srv.URL, Interval: time.Hour})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	rf.Run(ctx)

	if atomic.LoadInt32(calls) != 0 {
		t.Fatalf("Run refreshed even though the table was fetched recently (calls=%d)", *calls)
	}
}

func TestRunRefreshesOnStartWhenStale(t *testing.T) {
	db := newTestStore(t)
	if err := db.ReplaceIperf3Servers(context.Background(), []store.Iperf3Server{{Host: "existing.example.net", Port: 5201}}); err != nil {
		t.Fatal(err)
	}
	srv, calls := countingServer(t, oneServerFixture)

	rf := New(Config{
		Store: db, URL: srv.URL, Interval: time.Hour,
		Now: func() time.Time { return time.Now().Add(25 * time.Hour) },
	})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	rf.Run(ctx)

	if atomic.LoadInt32(calls) < 1 {
		t.Fatal("Run did not refresh a stale (>24h) table on start")
	}
}

func TestRunStopsOnContextCancel(t *testing.T) {
	db := newTestStore(t)
	srv, _ := countingServer(t, oneServerFixture)

	rf := New(Config{Store: db, URL: srv.URL, Interval: time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		rf.Run(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancel")
	}
}
