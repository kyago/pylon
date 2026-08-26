package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadVerifyConfig_Full(t *testing.T) {
	vc, err := LoadVerifyConfig(filepath.Join("../../testdata/config/verify.yml"))
	if err != nil {
		t.Fatalf("LoadVerifyConfig failed: %v", err)
	}

	if vc.Build == nil {
		t.Fatal("build should not be nil")
	}
	if vc.Build.Command != "go build ./..." {
		t.Errorf("build command = %q, want %q", vc.Build.Command, "go build ./...")
	}
	if vc.Build.Timeout != "120s" {
		t.Errorf("build timeout = %q, want %q", vc.Build.Timeout, "120s")
	}

	if vc.Test == nil {
		t.Fatal("test should not be nil")
	}
	if vc.Test.Command != "go test ./... -race" {
		t.Errorf("test command = %q, want %q", vc.Test.Command, "go test ./... -race")
	}

	if vc.Lint == nil {
		t.Fatal("lint should not be nil")
	}
	if vc.Lint.Command != "golangci-lint run ./..." {
		t.Errorf("lint command = %q, want %q", vc.Lint.Command, "golangci-lint run ./...")
	}
}

func TestLoadVerifyConfig_Partial(t *testing.T) {
	vc, err := LoadVerifyConfig(filepath.Join("../../testdata/config/verify_partial.yml"))
	if err != nil {
		t.Fatalf("LoadVerifyConfig failed: %v", err)
	}

	if vc.Build == nil {
		t.Fatal("build should not be nil")
	}
	if vc.Test == nil {
		t.Fatal("test should not be nil")
	}
	if vc.Lint != nil {
		t.Error("lint should be nil for partial config")
	}
}

func TestLoadVerifyConfig_NotFound(t *testing.T) {
	_, err := LoadVerifyConfig("/nonexistent/verify.yml")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestLoadVerifyConfig_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "verify.yml")
	os.WriteFile(path, []byte("{{invalid yaml"), 0644)

	_, err := LoadVerifyConfig(path)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestVerifyConfig_OrderedSteps_Full(t *testing.T) {
	vc := &VerifyConfig{
		Build: &VerifyStep{Command: "make build", Timeout: "120s"},
		Test:  &VerifyStep{Command: "make test", Timeout: "300s"},
		Lint:  &VerifyStep{Command: "make lint", Timeout: "60s"},
	}

	steps := vc.OrderedSteps()
	if len(steps) != 3 {
		t.Fatalf("got %d steps, want 3", len(steps))
	}

	// Order: build → test → lint
	if steps[0].Name != "build" {
		t.Errorf("steps[0].Name = %q, want build", steps[0].Name)
	}
	if steps[1].Name != "test" {
		t.Errorf("steps[1].Name = %q, want test", steps[1].Name)
	}
	if steps[2].Name != "lint" {
		t.Errorf("steps[2].Name = %q, want lint", steps[2].Name)
	}
}

func TestVerifyConfig_OrderedSteps_Partial(t *testing.T) {
	vc := &VerifyConfig{
		Build: &VerifyStep{Command: "make build"},
	}

	steps := vc.OrderedSteps()
	if len(steps) != 1 {
		t.Fatalf("got %d steps, want 1", len(steps))
	}
	if steps[0].Name != "build" {
		t.Errorf("steps[0].Name = %q, want build", steps[0].Name)
	}
	if steps[0].Timeout != "60s" {
		t.Errorf("steps[0].Timeout = %q, want 60s (default)", steps[0].Timeout)
	}
}

func TestVerifyConfig_OrderedSteps_Empty(t *testing.T) {
	vc := &VerifyConfig{}
	steps := vc.OrderedSteps()
	if len(steps) != 0 {
		t.Errorf("got %d steps, want 0", len(steps))
	}
}

func TestVerifyConfig_OrderedSteps_EmptyCommand(t *testing.T) {
	vc := &VerifyConfig{
		Build: &VerifyStep{Command: ""},
		Test:  &VerifyStep{Command: "make test"},
	}

	steps := vc.OrderedSteps()
	if len(steps) != 1 {
		t.Fatalf("got %d steps, want 1", len(steps))
	}
	if steps[0].Name != "test" {
		t.Errorf("steps[0].Name = %q, want test", steps[0].Name)
	}
}

func TestLoadVerifyConfig_CommandsSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "verify.yml")
	content := `commands:
  - name: build
    command: npm run build
    timeout: 5m
  - name: test
    command: npm test
    timeout: 10m
  - name: lint
    command: npm run lint
    timeout: 3m
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	vc, err := LoadVerifyConfig(path)
	if err != nil {
		t.Fatalf("LoadVerifyConfig failed: %v", err)
	}
	steps := vc.OrderedSteps()
	if len(steps) != 3 {
		t.Fatalf("got %d steps, want 3", len(steps))
	}
	if steps[0].Name != "build" || steps[0].Command != "npm run build" || steps[0].Timeout != "5m" {
		t.Errorf("unexpected build step: %+v", steps[0])
	}
	if steps[1].Name != "test" || steps[2].Name != "lint" {
		t.Errorf("unexpected command order: %+v", steps)
	}
}

func TestVerifyConfig_OrderedHeldOutSteps(t *testing.T) {
	vc := &VerifyConfig{HeldOut: []NamedVerifyStep{{Name: "acceptance", Command: "./acceptance.sh"}}}
	steps := vc.OrderedHeldOutSteps()
	if len(steps) != 1 {
		t.Fatalf("got %d held-out steps, want 1", len(steps))
	}
	if steps[0].Kind != VerifyKindHeldOut || steps[0].Timeout != "60s" {
		t.Fatalf("unexpected held-out step: %+v", steps[0])
	}
}

func TestLoadVerifyConfig_CommandsMapForm(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "verify.yml")
	content := `commands:
  build:
    command: go build ./...
    timeout: 5m
  test:
    command: go test ./...
held_out:
  acceptance:
    command: ./acceptance.sh
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	vc, err := LoadVerifyConfig(path)
	if err != nil {
		t.Fatalf("map-form verify.yml must parse: %v", err)
	}
	steps := vc.OrderedSteps()
	if len(steps) != 2 || steps[0].Name != "build" || steps[1].Name != "test" {
		t.Fatalf("map-form steps = %+v", steps)
	}
	if steps[0].Command != "go build ./..." || steps[0].Timeout != "5m" {
		t.Fatalf("map-form build step = %+v", steps[0])
	}
	if steps[1].Timeout != "60s" {
		t.Fatalf("default timeout not applied: %+v", steps[1])
	}
	heldOut := vc.OrderedHeldOutSteps()
	if len(heldOut) != 1 || heldOut[0].Name != "acceptance" || heldOut[0].Kind != VerifyKindHeldOut {
		t.Fatalf("map-form held_out = %+v", heldOut)
	}
}

func TestLoadVerifyConfig_CommandsMapFormExplicitNameWins(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "verify.yml")
	content := "commands:\n  build:\n    name: custom-build\n    command: make\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	vc, err := LoadVerifyConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	steps := vc.OrderedSteps()
	if len(steps) != 1 || steps[0].Name != "custom-build" {
		t.Fatalf("explicit name must win over map key: %+v", steps)
	}
}

func TestLoadVerifyConfig_CommandsScalarRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "verify.yml")
	if err := os.WriteFile(path, []byte("commands: run-everything\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadVerifyConfig(path); err == nil {
		t.Fatal("scalar commands value must be rejected")
	}
}
