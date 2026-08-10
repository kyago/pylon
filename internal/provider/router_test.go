package provider_test

import (
	"context"
	"strings"
	"testing"

	"github.com/kyago/pylon/internal/provider"
	"github.com/kyago/pylon/internal/provider/fake"
)

func TestRouterTaskOverridePrecedesWorkspaceProvider(t *testing.T) {
	registry := provider.NewRegistry()
	mustRegister(t, registry,
		fake.New("workspace-provider", provider.CapabilitySet{ReadFiles: true}),
		fake.New("task-provider", provider.CapabilitySet{ReadFiles: true}),
	)

	selection, err := provider.NewRouter(registry).Select(context.Background(), provider.SelectionRequest{
		TaskProvider:      "task-provider",
		WorkspaceProvider: "workspace-provider",
		Required:          provider.CapabilitySet{ReadFiles: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if selection.Adapter.Name() != "task-provider" || selection.Source != provider.SelectionSourceTaskOverride {
		t.Fatalf("selection = %s (%s)", selection.Adapter.Name(), selection.Source)
	}
}

func TestRouterWorkspaceProviderPrecedesAutoDetection(t *testing.T) {
	registry := provider.NewRegistry()
	mustRegister(t, registry,
		fake.New("auto-first", provider.CapabilitySet{ReadFiles: true, RunShell: true}),
		fake.New("workspace-provider", provider.CapabilitySet{ReadFiles: true, RunShell: true}),
	)

	selection, err := provider.NewRouter(registry).Select(context.Background(), provider.SelectionRequest{
		WorkspaceProvider: "workspace-provider",
		Required:          provider.CapabilitySet{RunShell: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if selection.Adapter.Name() != "workspace-provider" || selection.Source != provider.SelectionSourceWorkspace {
		t.Fatalf("selection = %s (%s)", selection.Adapter.Name(), selection.Source)
	}
}

func TestRouterAutoSelectsFirstCapabilityMatch(t *testing.T) {
	registry := provider.NewRegistry()
	mustRegister(t, registry,
		fake.New("read-only", provider.CapabilitySet{ReadFiles: true}),
		fake.New("shell", provider.CapabilitySet{ReadFiles: true, RunShell: true}),
	)

	selection, err := provider.NewRouter(registry).Select(context.Background(), provider.SelectionRequest{
		Required: provider.CapabilitySet{ReadFiles: true, RunShell: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if selection.Adapter.Name() != "shell" || selection.Source != provider.SelectionSourceAuto {
		t.Fatalf("selection = %s (%s)", selection.Adapter.Name(), selection.Source)
	}
}

func TestRouterExplicitProviderFailsOnMissingCapabilities(t *testing.T) {
	registry := provider.NewRegistry()
	mustRegister(t, registry, fake.New("read-only", provider.CapabilitySet{ReadFiles: true}))

	_, err := provider.NewRouter(registry).Select(context.Background(), provider.SelectionRequest{
		TaskProvider: "read-only",
		Required:     provider.CapabilitySet{RunShell: true},
	})
	if err == nil {
		t.Fatal("missing capability was accepted")
	}
	if !strings.Contains(err.Error(), "run_shell") || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("error does not explain missing capability: %v", err)
	}
}

func TestRouterReportsUnavailableProviders(t *testing.T) {
	registry := provider.NewRegistry()
	probeFailure := fake.New("broken", provider.CapabilitySet{})
	probeFailure.ProbeError = context.DeadlineExceeded
	mustRegister(t, registry, probeFailure)

	_, err := provider.NewRouter(registry).Select(context.Background(), provider.SelectionRequest{
		Required: provider.CapabilitySet{ReadFiles: true},
	})
	if err == nil {
		t.Fatal("router selected an unavailable provider")
	}
	if !strings.Contains(err.Error(), "broken") || !strings.Contains(err.Error(), "deadline") {
		t.Fatalf("error does not include probe failure: %v", err)
	}
}

func mustRegister(t *testing.T, registry *provider.Registry, adapters ...provider.Adapter) {
	t.Helper()
	for _, adapter := range adapters {
		if err := registry.Register(adapter); err != nil {
			t.Fatal(err)
		}
	}
}
