package provider

import (
	"context"
	"fmt"
	"strings"
)

type SelectionSource string

const (
	SelectionSourceTaskOverride SelectionSource = "task_override"
	SelectionSourceWorkspace    SelectionSource = "workspace"
	SelectionSourceAuto         SelectionSource = "auto"
)

type SelectionRequest struct {
	TaskProvider      string
	WorkspaceProvider string
	Required          CapabilitySet
}

type Selection struct {
	Adapter      Adapter
	Capabilities CapabilitySet
	Source       SelectionSource
}

type Router struct {
	registry *Registry
}

func NewRouter(registry *Registry) *Router {
	return &Router{registry: registry}
}

func (router *Router) Select(ctx context.Context, request SelectionRequest) (Selection, error) {
	if router == nil || router.registry == nil {
		return Selection{}, fmt.Errorf("provider registry is not configured")
	}
	if providerName := explicitProvider(request.TaskProvider); providerName != "" {
		return router.selectNamed(ctx, providerName, request.Required, SelectionSourceTaskOverride)
	}
	if providerName := explicitProvider(request.WorkspaceProvider); providerName != "" {
		return router.selectNamed(ctx, providerName, request.Required, SelectionSourceWorkspace)
	}

	var failures []string
	for _, adapter := range router.registry.Adapters() {
		capabilities, err := adapter.Probe(ctx)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", adapter.Name(), err))
			continue
		}
		if capabilities.Satisfies(request.Required) {
			return Selection{Adapter: adapter, Capabilities: capabilities, Source: SelectionSourceAuto}, nil
		}
		failures = append(failures, fmt.Sprintf("%s: missing %s", adapter.Name(), capabilityList(capabilities.Missing(request.Required))))
	}
	if len(failures) == 0 {
		failures = append(failures, "no providers registered")
	}
	return Selection{}, fmt.Errorf("no provider satisfies required capabilities: %s", strings.Join(failures, "; "))
}

func (router *Router) selectNamed(ctx context.Context, name string, required CapabilitySet, source SelectionSource) (Selection, error) {
	adapter, ok := router.registry.Get(name)
	if !ok {
		return Selection{}, fmt.Errorf("provider is not registered: %s", name)
	}
	capabilities, err := adapter.Probe(ctx)
	if err != nil {
		return Selection{}, fmt.Errorf("provider %s probe failed: %w", name, err)
	}
	if missing := capabilities.Missing(required); len(missing) > 0 {
		return Selection{}, fmt.Errorf("provider %s is missing required capabilities: %s", name, capabilityList(missing))
	}
	return Selection{Adapter: adapter, Capabilities: capabilities, Source: source}, nil
}

func explicitProvider(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || name == "auto" {
		return ""
	}
	return name
}

func capabilityList(capabilities []Capability) string {
	names := make([]string, len(capabilities))
	for index, capability := range capabilities {
		names[index] = string(capability)
	}
	return strings.Join(names, ", ")
}
