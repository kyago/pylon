package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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
	if !strings.Contains(err.Error(), ".pylon/config.yml") {
		t.Errorf("error should mention .pylon/config.yml: %v", err)
	}
}

func TestFindWorkspaceRoot_ConfigIsDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	pylonDir := filepath.Join(tmpDir, ".pylon")
	if err := os.MkdirAll(pylonDir, 0755); err != nil {
		t.Fatalf("failed to create .pylon/: %v", err)
	}
	// Create config.yml as a directory instead of a file
	if err := os.MkdirAll(filepath.Join(pylonDir, "config.yml"), 0755); err != nil {
		t.Fatalf("failed to create config.yml as dir: %v", err)
	}

	_, err := FindWorkspaceRoot(tmpDir)
	if err == nil {
		t.Fatal("expected error when config.yml is a directory, got nil")
	}
	if !strings.Contains(err.Error(), "not a regular file") {
		t.Errorf("error should mention 'not a regular file': %v", err)
	}
}

func TestFindWorkspaceRoot_PermissionDenied(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission test not reliable on Windows")
	}
	tmpDir := t.TempDir()
	pylonDir := filepath.Join(tmpDir, ".pylon")
	if err := os.MkdirAll(pylonDir, 0755); err != nil {
		t.Fatalf("failed to create .pylon/: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pylonDir, "config.yml"), []byte("version: \"0.1\"\n"), 0644); err != nil {
		t.Fatalf("failed to create config.yml: %v", err)
	}
	// Remove read permission on .pylon/
	if err := os.Chmod(pylonDir, 0000); err != nil {
		t.Fatalf("failed to chmod .pylon/: %v", err)
	}
	t.Cleanup(func() { os.Chmod(pylonDir, 0755) })

	_, err := FindWorkspaceRoot(tmpDir)
	if err == nil {
		t.Fatal("expected error for permission denied, got nil")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("error should mention permission denied: %v", err)
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

func TestDiscoverProjects_SkipsLinkedWorktrees(t *testing.T) {
	root := t.TempDir()
	source := t.TempDir()

	git := func(dir string, args ...string) {
		t.Helper()
		base := []string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false", "-c", "protocol.file.allow=always"}
		cmd := exec.Command("git", append(base, args...)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	for _, dir := range []string{root, source} {
		git(dir, "init")
		git(dir, "commit", "--allow-empty", "-m", "init")
	}

	if err := os.Mkdir(filepath.Join(root, "clone"), 0755); err != nil {
		t.Fatal(err)
	}
	git(filepath.Join(root, "clone"), "init")
	git(root, "submodule", "add", source, "sub")
	git(root, "worktree", "add", "wt")
	git(root, "-c", "worktree.useRelativePaths=true", "worktree", "add", "wt-rel")

	for _, name := range []string{"clone", "plain", "sub", "wt", "wt-rel"} {
		if err := os.MkdirAll(filepath.Join(root, name, ".pylon"), 0755); err != nil {
			t.Fatal(err)
		}
	}

	projects, err := DiscoverProjects(root)
	if err != nil {
		t.Fatalf("DiscoverProjects failed: %v", err)
	}

	var names []string
	for _, p := range projects {
		names = append(names, p.Name)
	}
	if got, want := strings.Join(names, ","), "clone,plain,sub"; got != want {
		t.Errorf("projects = %q, want %q", got, want)
	}
}
