package provider

import (
	"fmt"
	"strings"
	"sync"
)

type Registry struct {
	mu       sync.RWMutex
	adapters map[string]Adapter
	order    []string
}

func NewRegistry() *Registry {
	return &Registry{adapters: make(map[string]Adapter)}
}

func (registry *Registry) Register(adapter Adapter) error {
	if adapter == nil {
		return fmt.Errorf("provider adapter is nil")
	}
	name := strings.TrimSpace(adapter.Name())
	if name == "" || name == "auto" {
		return fmt.Errorf("invalid provider name: %q", name)
	}

	registry.mu.Lock()
	defer registry.mu.Unlock()
	if _, exists := registry.adapters[name]; exists {
		return fmt.Errorf("provider already registered: %s", name)
	}
	registry.adapters[name] = adapter
	registry.order = append(registry.order, name)
	return nil
}

func (registry *Registry) Get(name string) (Adapter, bool) {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	adapter, ok := registry.adapters[name]
	return adapter, ok
}

func (registry *Registry) Adapters() []Adapter {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	adapters := make([]Adapter, 0, len(registry.order))
	for _, name := range registry.order {
		adapters = append(adapters, registry.adapters[name])
	}
	return adapters
}
