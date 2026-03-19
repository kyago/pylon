package workflow

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadWorkflow_BuiltinFeature(t *testing.T) {
	wf, err := LoadWorkflow("feature", "")
	if err != nil {
		t.Fatalf("failed to load feature workflow: %v", err)
	}

	if wf.Name != "feature" {
		t.Errorf("expected name 'feature', got %q", wf.Name)
	}
	if len(wf.Stages) != 11 {
		t.Errorf("expected 11 stages, got %d", len(wf.Stages))
	}
	if wf.Stages[0] != StageInit {
		t.Errorf("expected first stage 'init', got %q", wf.Stages[0])
	}
	if wf.Stages[len(wf.Stages)-1] != StageCompleted {
		t.Errorf("expected last stage 'completed', got %q", wf.Stages[len(wf.Stages)-1])
	}
}

func TestLoadWorkflow_AllBuiltins(t *testing.T) {
	names := AvailableWorkflows()
	if len(names) < 7 {
		t.Fatalf("expected at least 7 builtin workflows, got %d: %v", len(names), names)
	}

	for _, name := range names {
		wf, err := LoadWorkflow(name, "")
		if err != nil {
			t.Errorf("failed to load builtin workflow %q: %v", name, err)
			continue
		}
		if wf.Name != name {
			t.Errorf("workflow %q: name mismatch, got %q", name, wf.Name)
		}
		if len(wf.Stages) < 2 {
			t.Errorf("workflow %q: must have at least 2 stages", name)
		}

		// All workflows must start with init and end with completed
		if wf.Stages[0] != StageInit {
			t.Errorf("workflow %q: must start with init, got %q", name, wf.Stages[0])
		}
		if wf.Stages[len(wf.Stages)-1] != StageCompleted {
			t.Errorf("workflow %q: must end with completed, got %q", name, wf.Stages[len(wf.Stages)-1])
		}

		// BuildTransitions should not panic
		transitions := wf.BuildTransitions()
		if len(transitions) == 0 {
			t.Errorf("workflow %q: BuildTransitions returned empty map", name)
		}
	}
}

func TestLoadWorkflow_CustomYAML(t *testing.T) {
	dir := t.TempDir()
	content := []byte(`name: custom
description: Custom test workflow
stages:
  - init
  - agent_executing
  - completed
`)
	if err := os.WriteFile(filepath.Join(dir, "custom.yml"), content, 0644); err != nil {
		t.Fatal(err)
	}

	wf, err := LoadWorkflow("custom", dir)
	if err != nil {
		t.Fatalf("failed to load custom workflow: %v", err)
	}

	if wf.Name != "custom" {
		t.Errorf("expected name 'custom', got %q", wf.Name)
	}
	if len(wf.Stages) != 3 {
		t.Errorf("expected 3 stages, got %d", len(wf.Stages))
	}
}

func TestLoadWorkflow_CustomOverridesBuiltin(t *testing.T) {
	dir := t.TempDir()
	// Custom feature.yml with fewer stages
	content := []byte(`name: feature
description: Custom feature workflow
stages:
  - init
  - agent_executing
  - completed
`)
	if err := os.WriteFile(filepath.Join(dir, "feature.yml"), content, 0644); err != nil {
		t.Fatal(err)
	}

	wf, err := LoadWorkflow("feature", dir)
	if err != nil {
		t.Fatal(err)
	}

	// Should use custom, not builtin
	if len(wf.Stages) != 3 {
		t.Errorf("expected custom feature with 3 stages, got %d (builtin was loaded instead)", len(wf.Stages))
	}
}

func TestLoadWorkflow_NotFound(t *testing.T) {
	_, err := LoadWorkflow("nonexistent", "")
	if err == nil {
		t.Error("expected error for nonexistent workflow")
	}
}

func TestAvailableWorkflows(t *testing.T) {
	names := AvailableWorkflows()

	expected := []string{"bugfix", "docs", "explore", "feature", "hotfix", "refactor", "review"}
	if len(names) != len(expected) {
		t.Fatalf("expected %d workflows, got %d: %v", len(expected), len(names), names)
	}

	for _, e := range expected {
		found := false
		for _, n := range names {
			if n == e {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected workflow %q not found in %v", e, names)
		}
	}
}
