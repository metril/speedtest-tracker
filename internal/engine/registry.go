package engine

import (
	"sort"
	"sync"
)

// Registry maps engine names to engines. It is safe for concurrent use:
// the runner reads it while the settings watcher rebuilds it whenever an
// engines.* setting changes.
type Registry struct {
	mu      sync.RWMutex
	engines map[string]Engine
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{engines: map[string]Engine{}}
}

// Register adds e, replacing any engine with the same name.
func (r *Registry) Register(e Engine) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.engines[e.Name()] = e
}

// Replace swaps the whole engine set atomically. Callers build a fresh map
// (never mutate the one they passed in afterwards).
func (r *Registry) Replace(engines map[string]Engine) {
	next := make(map[string]Engine, len(engines))
	for name, e := range engines {
		next[name] = e
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.engines = next
}

// Get returns the engine registered under name.
func (r *Registry) Get(name string) (Engine, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.engines[name]
	return e, ok
}

// Names returns the registered engine names in sorted order.
func (r *Registry) Names() []string {
	r.mu.RLock()
	names := make([]string, 0, len(r.engines))
	for n := range r.engines {
		names = append(names, n)
	}
	r.mu.RUnlock()
	sort.Strings(names)
	return names
}
