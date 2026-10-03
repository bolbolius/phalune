package widget

import (
	"fmt"
	"strings"
	"sync"
)

type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
	// prefixes are matched after exact names; longest prefix wins.
	prefixes map[string]Factory
}

func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]Factory),
		prefixes:  make(map[string]Factory),
	}
}

func (r *Registry) Register(name string, factory Factory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[name] = factory
}

// RegisterPrefix registers a factory for every widget name starting with
// prefix (e.g. "custom." for user script widgets).
func (r *Registry) RegisterPrefix(prefix string, factory Factory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.prefixes[prefix] = factory
}

func (r *Registry) Create(name string, ctx Context) (Widget, error) {
	rest := ""
	r.mu.RLock()
	factory, ok := r.factories[name]
	if !ok {
		for prefix, f := range r.prefixes {
			if strings.HasPrefix(name, prefix) {
				// Longest matching prefix wins.
				if len(prefix) > len(rest) {
					factory, ok, rest = f, true, strings.TrimPrefix(name, prefix)
				}
			}
		}
	}
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("unknown widget %q", name)
	}

	ctx.WidgetName = rest
	w, err := factory(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create widget %q: %w", name, err)
	}

	return w, nil
}

func (r *Registry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if _, ok := r.factories[name]; ok {
		return true
	}
	for prefix := range r.prefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}
