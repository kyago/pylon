package claude

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/kyago/pylon/internal/provider"
)

func TestAdapterProbeAndVersionUseConfiguredCommand(t *testing.T) {
	var lookedUp string
	var versionPath string
	adapter := New("custom-claude",
		WithLookPath(func(command string) (string, error) {
			lookedUp = command
			return "/opt/bin/custom-claude", nil
		}),
		WithVersionCommand(func(_ context.Context, path string) ([]byte, error) {
			versionPath = path
			return []byte("1.2.3\n"), nil
		}),
	)

	capabilities, err := adapter.Probe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if lookedUp != "custom-claude" || !capabilities.ReadFiles || !capabilities.SessionResume {
		t.Fatalf("probe = command %q, capabilities %+v", lookedUp, capabilities)
	}
	version, err := adapter.Version(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if version != "1.2.3" || versionPath != "/opt/bin/custom-claude" {
		t.Fatalf("version = %q via %q", version, versionPath)
	}
}

func TestAdapterProbeReportsMissingExecutable(t *testing.T) {
	want := errors.New("missing")
	adapter := New("custom-claude", WithLookPath(func(string) (string, error) {
		return "", want
	}))
	if _, err := adapter.Probe(context.Background()); !errors.Is(err, want) {
		t.Fatalf("Probe() error = %v", err)
	}
}

func TestPrepareInteractiveBuildsProcessSpec(t *testing.T) {
	adapter := New("custom-claude", WithLookPath(func(string) (string, error) {
		return "/opt/bin/custom-claude", nil
	}))

	process, err := adapter.PrepareInteractive(context.Background(), provider.InteractiveSpec{
		MaxTurns:       25,
		PermissionMode: "acceptEdits",
		Environment:    []string{"PATH=/usr/bin", "LEVEL=old"},
		EnvironmentOverride: map[string]string{
			"LEVEL": "high",
			"TOKEN": "set",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if process.Executable != "/opt/bin/custom-claude" || process.DisplayName != "Claude Code" {
		t.Fatalf("process = %+v", process)
	}
	wantArgs := []string{"custom-claude", "--max-turns", "25", "--permission-mode", "acceptEdits"}
	if !reflect.DeepEqual(process.Args, wantArgs) {
		t.Fatalf("args = %v, want %v", process.Args, wantArgs)
	}
	wantEnvironment := []string{"LEVEL=high", "PATH=/usr/bin", "TOKEN=set"}
	if !reflect.DeepEqual(process.Environment, wantEnvironment) {
		t.Fatalf("environment = %v, want %v", process.Environment, wantEnvironment)
	}
}

func TestManagedTaskLifecycleIsExplicitlyUnsupported(t *testing.T) {
	adapter := New("claude", WithLookPath(func(string) (string, error) { return "/bin/claude", nil }))
	if _, err := adapter.Start(context.Background(), provider.TaskSpec{}); !errors.Is(err, provider.ErrUnsupportedOperation) {
		t.Fatalf("Start() error = %v", err)
	}
}
