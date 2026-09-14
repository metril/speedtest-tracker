package ooklaweb

import (
	"fmt"
	"sync"

	"github.com/metril/speedtest-tracker/internal/engine/ookla"
)

// singleflightGroup dedupes concurrent calls sharing the same key: the
// first caller for a key ("the leader") runs fn; every other concurrent
// caller for the same key blocks and receives the leader's result instead
// of running fn itself. This is a small, purpose-built stand-in for
// golang.org/x/sync/singleflight.Group (not a project dependency); the
// zero value is ready to use.
type singleflightGroup struct {
	mu    sync.Mutex
	calls map[string]*singleflightCall
}

type singleflightCall struct {
	wg      sync.WaitGroup
	servers []ookla.Server
	err     error
}

// Do runs fn, or waits for and shares the result of an already-in-flight
// call for the same key.
//
// A panicking fn is recovered just long enough to release every waiter
// (with the call's zero value and the recovered panic re-wrapped into
// err) before being re-panicked in the leader's own goroutine, mirroring
// golang.org/x/sync/singleflight.Group's behavior. Without this, a
// panicking leader would leave every waiter blocked on c.wg.Wait()
// forever, since neither c.wg.Done() nor the map cleanup below would ever
// run.
func (g *singleflightGroup) Do(key string, fn func() ([]ookla.Server, error)) ([]ookla.Server, error) {
	g.mu.Lock()
	if c, ok := g.calls[key]; ok {
		g.mu.Unlock()
		c.wg.Wait()
		return c.servers, c.err
	}
	c := &singleflightCall{}
	c.wg.Add(1)
	if g.calls == nil {
		g.calls = make(map[string]*singleflightCall)
	}
	g.calls[key] = c
	g.mu.Unlock()

	g.doCall(c, key, fn)

	return c.servers, c.err
}

// doCall runs fn for c, always releasing c's waiters and removing c from
// the group afterward — including when fn panics, in which case the panic
// is re-raised in this (the leader's) goroutine only, after every waiter
// has been unblocked.
func (g *singleflightGroup) doCall(c *singleflightCall, key string, fn func() ([]ookla.Server, error)) {
	normalReturn := false
	defer func() {
		if !normalReturn {
			if r := recover(); r != nil {
				c.err = fmt.Errorf("ooklaweb: panic in single-flighted call: %v", r)
				c.wg.Done()

				g.mu.Lock()
				delete(g.calls, key)
				g.mu.Unlock()

				panic(r)
			}
		}
	}()

	c.servers, c.err = fn()
	normalReturn = true
	c.wg.Done()

	g.mu.Lock()
	delete(g.calls, key)
	g.mu.Unlock()
}
