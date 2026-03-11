package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindWorkspaceRoot_Found(t *testing.T) {
	// Create a temp directory structure with .pylon/ containing config.yml
	tmpDir := t.TempDir()
	pylonDir := filepath.Join(tmpDir, ".pylon")
	if err := os.MkdirAll(pylonDir, 0755); err != nil {
		t.Fatalf("failed to create .pylon/: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pylonDir, "config.yml"), []byte("version: \"0.1\"\n"), 0644); err != nil {
		t.Fatalf("failed to create config.yml: %v", err)
	}

	// Create a nested subdirectory
	subDir := filepath.Join(tmpDir, "sub", "deep")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("failed to create subdirectory: %v", err)
	}

	// Search from the nested subdirectory
	root, err := FindWorkspaceRoot(subDir)
	if err != nil {
		t.Fatalf("FindWorkspaceRoot failed: %v", err)
	}

	if root != tmpDir {
		t.Errorf("expected root %q, got %q", tmpDir, root)
	}
}

func TestFindWorkspaceRoot_SameDir(t *testing.T) {
	tmpDir := t.TempDir()
	pylonDir := filepath.Join(tmpDir, ".pylon")
	if err := os.MkdirAll(pylonDir, 0755); err != nil {
		t.Fatalf("failed to create .pylon/: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pylonDir, "config.yml"), []byte("version: \"0.1\"\n"), 0644); err != nil {
		t.Fatalf("failed to create config.yml: %v", err)
	}

	root, err := FindWorkspaceRoot(tmpDir)
	if err != nil {
		t.Fatalf("FindWorkspaceRoot failed: %v", err)
	}

	if root != tmpDir {
		t.Errorf("expected root %q, got %q", tmpDir, root)
	}
}

func TestFindWorkspaceRoot_SubProjectFallback(t *testing.T) {
	// Create workspace root with .pylon/config.yml
	tmpDir := t.TempDir()
	rootPylonDir := filepath.Join(tmpDir, ".pylon")
	if err := os.MkdirAll(rootPylonDir, 0755); err != nil {
		t.Fatalf("failed to create root .pylon/: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rootPylonDir, "config.yml"), []byte("version: \"0.1\"\n"), 0644); err != nil {
		t.Fatalf("failed to create config.yml: %v", err)
	}

	// Create sub-project with .pylon/ but WITHOUT config.yml
	subProjectDir := filepath.Join(tmpDir, "santa-backoffice")
	subPylonDir := filepath.Join(subProjectDir, ".pylon")
	if err := os.MkdirAll(subPylonDir, 0755); err != nil {
		t.Fatalf("failed to create sub-project .pylon/: %v", err)
	}
	// Sub-project has context.md and agents/ but no config.yml
	if err := os.WriteFile(filepath.Join(subPylonDir, "context.md"), []byte("# Context"), 0644); err != nil {
		t.Fatalf("failed to create context.md: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(subPylonDir, "agents"), 0755); err != nil {
		t.Fatalf("failed to create agents/: %v", err)
	}

	// Search from sub-project directory should find root workspace
	root, err := FindWorkspaceRoot(subProjectDir)
	if err != nil {
		t.Fatalf("FindWorkspaceRoot failed: %v", err)
	}

	if root != tmpDir {
		t.Errorf("expected root %q, got %q", tmpDir, root)
	}
}

func TestFindWorkspaceRoot_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	// No .pylon/ directory

	_, err := FindWorkspaceRoot(tmpDir)
	if err == nil {
		t.Fatal("expected error when no .pylon/ exists, got nil")
	}
}

func TestDiscoverProjects(t *testing.T) {
	tmpDir := t.TempDir()

	// Create root .pylon/
	if err := os.MkdirAll(filepath.Join(tmpDir, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create project-api with .pylon/
	if err := os.MkdirAll(filepath.Join(tmpDir, "project-api", ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create project-web with .pylon/
	if err := os.MkdirAll(filepath.Join(tmpDir, "project-web", ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create a non-project directory (no .pylon/)
	if err := os.MkdirAll(filepath.Join(tmpDir, "docs"), 0755); err != nil {
		t.Fatal(err)
	}

	projects, err := DiscoverProjects(tmpDir)
	if err != nil {
		t.Fatalf("DiscoverProjects failed: %v", err)
	}

	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}

	// Check project names (order may vary)
	names := make(map[string]bool)
	for _, p := range projects {
		names[p.Name] = true
	}

	if !names["project-api"] {
		t.Error("expected project-api to be discovered")
	}
	if !names["project-web"] {
		t.Error("expected project-web to be discovered")
	}
}

func TestDiscoverProjects_NoProjects(t *testing.T) {
	tmpDir := t.TempDir()

	projects, err := DiscoverProjects(tmpDir)
	if err != nil {
		t.Fatalf("DiscoverProjects failed: %v", err)
	}

	if len(projects) != 0 {
		t.Errorf("expected 0 projects, got %d", len(projects))
	}
}

func TestLoadAllAgents(t *testing.T) {
	tmpDir := t.TempDir()

	// Create root agent directory
	agentsDir := filepath.Join(tmpDir, ".pylon", "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a root agent
	agentContent := `---
name: po
role: Product Owner
---

# PO Agent
`
	if err := os.WriteFile(filepath.Join(agentsDir, "po.md"), []byte(agentContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a project with its own agent
	projectAgentsDir := filepath.Join(tmpDir, "project-api", ".pylon", "agents")
	if err := os.MkdirAll(projectAgentsDir, 0755); err != nil {
		t.Fatal(err)
	}

	projectAgentContent := `---
name: backend-dev
role: Backend Developer
---

# Backend Dev
`
	if err := os.WriteFile(filepath.Join(projectAgentsDir, "backend-dev.md"), []byte(projectAgentContent), 0644); err != nil {
		t.Fatal(err)
	}

	agents, err := LoadAllAgents(tmpDir)
	if err != nil {
		t.Fatalf("LoadAllAgents failed: %v", err)
	}

	if len(agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(agents))
	}

	if _, ok := agents["po"]; !ok {
		t.Error("expected agent 'po' to be loaded")
	}
	if _, ok := agents["backend-dev"]; !ok {
		t.Error("expected agent 'backend-dev' to be loaded")
	}
}

func TestLoadAllAgents_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .pylon/agents/ but no agent files
	agentsDir := filepath.Join(tmpDir, ".pylon", "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatal(err)
	}

	agents, err := LoadAllAgents(tmpDir)
	if err != nil {
		t.Fatalf("LoadAllAgents failed: %v", err)
	}

	if len(agents) != 0 {
		t.Errorf("expected 0 agents, got %d", len(agents))
	}
}
