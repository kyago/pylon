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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// poisonGitEnv sets everything outside a repository that can change what git reports about it.
func poisonGitEnv(t *testing.T) {
	t.Helper()
	global := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(global, []byte("[core]\n\tbare = false\n"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	t.Setenv("GIT_CONFIG", global)
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "core.bare")
	t.Setenv("GIT_CONFIG_VALUE_0", "false")
	t.Setenv("GIT_WORK_TREE", t.TempDir())
	t.Setenv("GIT_COMMON_DIR", filepath.Join(t.TempDir(), "missing"))
}

func TestDiscoverProjects_SkipsWorkspaceWorktrees(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := os.Mkdir(root, 0755); err != nil {
		t.Fatal(err)
	}
	source := t.TempDir()
	bare := filepath.Join(t.TempDir(), "bare.git")

	git := func(dir string, args ...string) {
		t.Helper()
		base := []string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false", "-c", "protocol.file.allow=always"}
		cmd := exec.Command("git", append(base, args...)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	clone := filepath.Join(root, "clone")
	if err := os.Mkdir(clone, 0755); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{root, source, clone} {
		git(dir, "init")
		git(dir, "commit", "--allow-empty", "-m", "init")
	}
	git(source, "clone", "--bare", source, bare)

	git(root, "submodule", "add", source, "sub")
	git(root, "worktree", "add", "wt")
	git(root, "worktree", "add", "wt-rel")
	git(clone, "worktree", "add", filepath.Join(root, "clone-wt"))
	git(source, "worktree", "add", filepath.Join(root, "external"))
	git(bare, "worktree", "add", filepath.Join(root, "bare-wt"))
	// Hand-edited but valid spellings of a non-bare repository.
	git(root, "clone", source, "app-no")
	git(filepath.Join(root, "app-no"), "config", "core.bare", "no")
	git(filepath.Join(root, "app-no"), "worktree", "add", filepath.Join(root, "no-wt"))
	git(root, "clone", source, "app-dup")
	git(filepath.Join(root, "app-dup"), "config", "--add", "core.bare", "false")
	git(filepath.Join(root, "app-dup"), "worktree", "add", filepath.Join(root, "dup-wt"))
	// Written by hand because older git ignores worktree.useRelativePaths.
	if err := os.WriteFile(filepath.Join(root, "wt-rel", ".git"), []byte("gitdir: ../.git/worktrees/wt-rel\n"), 0644); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"app-dup", "app-no", "bare-wt", "clone", "clone-wt", "dup-wt", "external", "no-wt", "plain", "sub", "wt", "wt-rel"} {
		if err := os.MkdirAll(filepath.Join(root, name, ".pylon"), 0755); err != nil {
			t.Fatal(err)
		}
	}

	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}

	join := func(projects []ProjectInfo) string {
		var names []string
		for _, p := range projects {
			names = append(names, p.Name)
		}
		return strings.Join(names, ",")
	}
	dirs := []string{root, alias}
	// The same directory spelled differently on a case-insensitive filesystem (APFS default).
	if variant := filepath.Join(filepath.Dir(root), "WORKSPACE"); fileExists(variant) {
		dirs = append(dirs, variant)
	}
	check := func() {
		t.Helper()
		for _, dir := range dirs {
			projects, err := DiscoverProjects(dir)
			if err != nil {
				t.Fatalf("DiscoverProjects(%s) failed: %v", dir, err)
			}
			if got, want := join(projects), "app-dup,app-no,bare-wt,clone,external,plain,sub"; got != want {
				t.Errorf("DiscoverProjects(%s) = %q, want %q", dir, got, want)
			}
			worktrees, err := DiscoverWorktrees(dir)
			if err != nil {
				t.Fatalf("DiscoverWorktrees(%s) failed: %v", dir, err)
			}
			if got, want := join(worktrees), "clone-wt,dup-wt,no-wt,wt,wt-rel"; got != want {
				t.Errorf("DiscoverWorktrees(%s) = %q, want %q", dir, got, want)
			}
		}
	}
	check()
	poisonGitEnv(t)
	check()
}

func TestDiscoverProjects_KeepsWorktreesWithoutWorkspaceCheckout(t *testing.T) {
	root := t.TempDir()
	source := t.TempDir()

	git := func(dir string, args ...string) {
		t.Helper()
		base := []string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}
		cmd := exec.Command("git", append(base, args...)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git(source, "init")
	git(source, "commit", "--allow-empty", "-m", "init")

	// Bare repositories named .git, at the root and in a subdirectory: their worktrees are the only checkouts.
	git(root, "clone", "--bare", source, ".git")
	git(root, "worktree", "add", "from-root-bare")
	git(root, "clone", "--bare", source, filepath.Join("app", ".git"))
	git(filepath.Join(root, "app", ".git"), "worktree", "add", filepath.Join(root, "from-sub-bare"))
	// A hidden directory is never discovered as a project, so its worktree stays one.
	git(root, "clone", source, ".hub")
	git(filepath.Join(root, ".hub"), "worktree", "add", filepath.Join(root, "from-hidden"))

	// core.worktree puts the only other checkout outside the workspace.
	git(root, "clone", source, "core-wt")
	git(filepath.Join(root, "core-wt"), "worktree", "add", filepath.Join(root, "from-core-wt"))
	git(filepath.Join(root, "core-wt"), "config", "core.worktree", t.TempDir())

	// A separate git dir: its checkout is elsewhere.
	separate := t.TempDir()
	git(separate, "init", "--separate-git-dir", filepath.Join(root, "other.git"), ".")
	git(separate, "commit", "--allow-empty", "-m", "init")
	git(separate, "worktree", "add", filepath.Join(root, "from-separate"))

	for _, name := range []string{"from-root-bare", "from-sub-bare", "from-hidden", "from-core-wt", "from-separate"} {
		if err := os.MkdirAll(filepath.Join(root, name, ".pylon"), 0755); err != nil {
			t.Fatal(err)
		}
	}

	poisonGitEnv(t)

	projects, err := DiscoverProjects(root)
	if err != nil {
		t.Fatalf("DiscoverProjects failed: %v", err)
	}
	var names []string
	for _, p := range projects {
		names = append(names, p.Name)
	}
	if got, want := strings.Join(names, ","), "from-core-wt,from-hidden,from-root-bare,from-separate,from-sub-bare"; got != want {
		t.Errorf("projects = %q, want %q", got, want)
	}
}
