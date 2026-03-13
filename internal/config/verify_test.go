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
