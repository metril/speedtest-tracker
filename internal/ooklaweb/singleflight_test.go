package ooklaweb

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/metril/speedtest-tracker/internal/engine/ookla"
)

// TestSingleflightGroupRecoversPanicAndReleasesWaiters is a regression test
// for a panicking leader: without recover/re-panic in Do, a waiter blocked
// on c.wg.Wait() would hang forever, since neither c.wg.Done() nor the
// call's removal from the group would ever run.
func TestSingleflightGroupRecoversPanicAndReleasesWaiters(t *testing.T) {
	var g singleflightGroup
	started := make(chan struct{})
	release := make(chan struct{})

	var leaderWG sync.WaitGroup
	leaderWG.Add(1)
	var leaderPanic any
	go func() {
		defer leaderWG.Done()
		defer func() { leaderPanic = recover() }()
		_, _ = g.Do("k", func() (SearchResult, error) {
			close(started)
			<-release
			panic("boom")
		})
	}()

	<-started // the leader is now registered as the in-flight call for "k"

	waiterDone := make(chan struct{})
	var waiterErr error
	go func() {
		defer close(waiterDone)
		_, waiterErr = g.Do("k", func() (SearchResult, error) {
			t.Error("the waiter's own fn ran; it should have shared the leader's in-flight call instead")
			return SearchResult{}, nil
		})
	}()

	// Give the waiter goroutine time to actually reach Do and block on
	// c.wg.Wait() before letting the leader panic.
	time.Sleep(20 * time.Millisecond)
	close(release)

	select {
	case <-waiterDone:
	case <-time.After(2 * time.Second):
		t.Fatal("waiter was never released after the leader's fn panicked")
	}
	leaderWG.Wait()

	if leaderPanic == nil {
		t.Fatal("the leader's goroutine did not observe the re-raised panic")
	}
	if leaderPanic != "boom" {
		t.Errorf("leader panic value = %v, want the original \"boom\"", leaderPanic)
	}
	if waiterErr == nil || !strings.Contains(waiterErr.Error(), "panic") {
		t.Fatalf("waiter err = %v, want a non-nil error mentioning the panic", waiterErr)
	}

	// The panicked call must have been removed from the group so a later
	// Do for the same key runs fresh rather than reusing it.
	got, err := g.Do("k", func() (SearchResult, error) { return SearchResult{Servers: []ookla.Server{{ID: "1"}}}, nil })
	if err != nil || len(got.Servers) != 1 || got.Servers[0].ID != "1" {
		t.Fatalf("Do after panic = %+v, %v, want a fresh successful call", got, err)
	}
}
