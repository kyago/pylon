package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/provider"
)

type testInteractiveAdapter struct {
	name       string
	process    provider.ProcessSpec
	probeError error
	lastSpec   provider.InteractiveSpec
}

func (adapter *testInteractiveAdapter) Name() string {
	return adapter.name
}

func (adapter *testInteractiveAdapter) Probe(context.Context) (provider.CapabilitySet, error) {
	return provider.CapabilitySet{ReadFiles: true}, adapter.probeError
}

func (adapter *testInteractiveAdapter) PrepareInteractive(_ context.Context, spec provider.InteractiveSpec) (provider.ProcessSpec, error) {
	adapter.lastSpec = spec
	return adapter.process, nil
}

func (adapter *testInteractiveAdapter) Start(context.Context, provider.TaskSpec) (provider.WorkerHandle, error) {
	return provider.WorkerHandle{}, provider.ErrUnsupportedOperation
}

func (adapter *testInteractiveAdapter) Poll(context.Context, provider.WorkerHandle) (provider.WorkerStatus, error) {
	return provider.WorkerStatus{}, provider.ErrUnsupportedOperation
}

func (adapter *testInteractiveAdapter) Resume(context.Context, provider.WorkerHandle) (provider.WorkerHandle, error) {
	return provider.WorkerHandle{}, provider.ErrUnsupportedOperation
}

func (adapter *testInteractiveAdapter) Cancel(context.Context, provider.WorkerHandle) error {
	return provider.ErrUnsupportedOperation
}

func (adapter *testInteractiveAdapter) Collect(context.Context, provider.WorkerHandle) (provider.TaskResult, error) {
	return provider.TaskResult{}, provider.ErrUnsupportedOperation
}

func TestProviderEnabled(t *testing.T) {
	for _, value := range []string{"", "auto", "true", "enabled"} {
		enabled, err := providerEnabled(value)
		if err != nil || !enabled {
			t.Fatalf("providerEnabled(%q) = %v, %v", value, enabled, err)
		}
	}
	for _, value := range []string{"false", "disabled"} {
		enabled, err := providerEnabled(value)
		if err != nil || enabled {
			t.Fatalf("providerEnabled(%q) = %v, %v", value, enabled, err)
		}
	}
	if _, err := providerEnabled("sometimes"); err == nil {
		t.Fatal("invalid provider enabled value was accepted")
	}
}

func TestRunLaunchUsesSelectedInteractiveProvider(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".pylon", "config.yml"), []byte(`version: "0.2"
runtime:
  provider: provider-b
  max_turns: 17
  permission_mode: default
`), 0644); err != nil {
		t.Fatal(err)
	}

	adapter := &testInteractiveAdapter{
		name: "provider-b",
		process: provider.ProcessSpec{
			Executable:  "/opt/provider-b",
			Args:        []string{"provider-b", "interactive"},
			Environment: []string{"MODE=test"},
			DisplayName: "Provider B",
		},
	}
	catalog := &providerCatalog{registry: provider.NewRegistry(), entries: make(map[string]providerCatalogEntry)}
	if err := catalog.register(providerCatalogEntry{
		adapter: adapter,
		prepareWorkspace: func(string, *config.Config, []config.ProjectInfo) error {
			return nil
		},
		selectPermission: func(string) (string, error) {
			return "sandbox", nil
		},
	}); err != nil {
		t.Fatal(err)
	}

	oldWorkspace := flagWorkspace
	oldCatalogFactory := buildLaunchProviderCatalog
	oldReplaceProcess := replaceLaunchProcess
	flagWorkspace = root
	buildLaunchProviderCatalog = func(*config.Config) (*providerCatalog, error) { return catalog, nil }
	var gotExecutable string
	var gotArgs, gotEnvironment []string
	replaceLaunchProcess = func(executable string, args []string, environment []string) error {
		gotExecutable = executable
		gotArgs = append([]string(nil), args...)
		gotEnvironment = append([]string(nil), environment...)
		return nil
	}
	t.Cleanup(func() {
		flagWorkspace = oldWorkspace
		buildLaunchProviderCatalog = oldCatalogFactory
		replaceLaunchProcess = oldReplaceProcess
	})

	if err := runLaunch(); err != nil {
		t.Fatal(err)
	}
	if gotExecutable != adapter.process.Executable || !reflect.DeepEqual(gotArgs, adapter.process.Args) || !reflect.DeepEqual(gotEnvironment, adapter.process.Environment) {
		t.Fatalf("exec = %q %v env=%v", gotExecutable, gotArgs, gotEnvironment)
	}
	if adapter.lastSpec.MaxTurns != 17 || adapter.lastSpec.PermissionMode != "sandbox" {
		t.Fatalf("interactive spec = %+v", adapter.lastSpec)
	}
}

func TestProviderCatalogDoesNotFallbackFromExplicitUnknownProvider(t *testing.T) {
	cfg := &config.Config{Runtime: config.RuntimeConfig{Provider: "provider-b"}}
	catalog, err := newProviderCatalog(cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = catalog.selectInteractive(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "provider-b") || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("selection error = %v", err)
	}
}

func TestProviderCatalogReportsProbeFailure(t *testing.T) {
	want := errors.New("unavailable")
	adapter := &testInteractiveAdapter{name: "provider-b", probeError: want}
	catalog := &providerCatalog{registry: provider.NewRegistry(), entries: make(map[string]providerCatalogEntry)}
	if err := catalog.register(providerCatalogEntry{adapter: adapter}); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Runtime: config.RuntimeConfig{Provider: "provider-b"}}
	_, _, err := catalog.selectInteractive(context.Background(), cfg)
	if !errors.Is(err, want) {
		t.Fatalf("selection error = %v", err)
	}
}

func TestProviderCatalogRejectsNilConfig(t *testing.T) {
	if _, err := newProviderCatalog(nil); err == nil {
		t.Fatal("nil provider config was accepted")
	}
}
