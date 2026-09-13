package engine

import "sort"

// Registry maps engine names to engines. It is populated once at startup
// and read-only afterwards.
type Registry struct {
	engines map[string]Engine
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{engines: map[string]Engine{}}
}

// Register adds e, replacing any engine with the same name.
func (r *Registry) Register(e Engine) {
	r.engines[e.Name()] = e
}

// Get returns the engine registered under name.
func (r *Registry) Get(name string) (Engine, bool) {
	e, ok := r.engines[name]
	return e, ok
}

// Names returns the registered engine names in sorted order.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.engines))
	for n := range r.engines {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
