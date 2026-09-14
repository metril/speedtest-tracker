package ooklaweb

import (
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

	c.servers, c.err = fn()
	c.wg.Done()

	g.mu.Lock()
	delete(g.calls, key)
	g.mu.Unlock()

	return c.servers, c.err
}
