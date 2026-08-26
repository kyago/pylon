package codex

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
	adapter := New("custom-codex",
		WithLookPath(func(command string) (string, error) {
			lookedUp = command
			return "/opt/bin/custom-codex", nil
		}),
		WithVersionCommand(func(_ context.Context, path string) ([]byte, error) {
			versionPath = path
			return []byte("codex-cli 0.75.0\n"), nil
		}),
	)

	capabilities, err := adapter.Probe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if lookedUp != "custom-codex" || !capabilities.ReadFiles || !capabilities.RunShell {
		t.Fatalf("probe = command %q, capabilities %+v", lookedUp, capabilities)
	}
	version, err := adapter.Version(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if version != "codex-cli 0.75.0" || versionPath != "/opt/bin/custom-codex" {
		t.Fatalf("version = %q via %q", version, versionPath)
	}
}

func TestAdapterProbeReportsMissingExecutable(t *testing.T) {
	want := errors.New("missing")
	adapter := New("custom-codex", WithLookPath(func(string) (string, error) {
		return "", want
	}))
	if _, err := adapter.Probe(context.Background()); !errors.Is(err, want) {
		t.Fatalf("Probe() error = %v", err)
	}
}

func TestPrepareInteractivePassesSandboxMode(t *testing.T) {
	adapter := New("", WithLookPath(func(command string) (string, error) {
		return "/usr/local/bin/" + command, nil
	}))
	process, err := adapter.PrepareInteractive(context.Background(), provider.InteractiveSpec{
		PermissionMode: "read-only",
		Environment:    []string{"A=1"},
		EnvironmentOverride: map[string]string{
			"B": "2",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if process.Executable != "/usr/local/bin/codex" || process.DisplayName != "Codex" {
		t.Fatalf("process = %+v", process)
	}
	if !reflect.DeepEqual(process.Args, []string{"codex", "--sandbox", "read-only"}) {
		t.Fatalf("args = %v", process.Args)
	}
	if !reflect.DeepEqual(process.Environment, []string{"A=1", "B=2"}) {
		t.Fatalf("environment = %v", process.Environment)
	}
}

func TestPrepareInteractiveOmitsUnknownPermissionMode(t *testing.T) {
	adapter := New("", WithLookPath(func(command string) (string, error) {
		return "/usr/local/bin/" + command, nil
	}))
	// claude 전용 permission mode는 codex 플래그로 넘기지 않는다.
	process, err := adapter.PrepareInteractive(context.Background(), provider.InteractiveSpec{
		PermissionMode: "acceptEdits",
		MaxTurns:       10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(process.Args, []string{"codex"}) {
		t.Fatalf("args = %v", process.Args)
	}
}
