package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupSubmoduleFixture builds a workspace with one submodule pointing at a
// freshly-initialized origin repo. Returns workspace path, project name, and
// the submodule's working tree path.
func setupSubmoduleFixture(t *testing.T) (workspace, name, subPath string) {
	t.Helper()
	requireGit(t)
	name = "sub"

	// Origin repo (non-bare is fine for file:// clones)
	origin := t.TempDir()
	runGit(t, origin, "init")
	if err := os.WriteFile(filepath.Join(origin, "README.md"), []byte("hi\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, origin, "add", ".")
	runGit(t, origin, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-m", "init")

	// Workspace as superproject — needs .pylon/config.yml for FindWorkspaceRoot
	workspace = t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".pylon", "config.yml"), []byte("version: \"0.1\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, workspace, "init")
	runGit(t, workspace, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "--allow-empty", "-m", "ws init")
	runGit(t, workspace, "-c", "protocol.file.allow=always", "submodule", "add", "file://"+origin, name)
	runGit(t, workspace, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-m", "add submodule")

	subPath = filepath.Join(workspace, name)
	return workspace, name, subPath
}

func TestSafetyCheck_DirtyWorkingTree(t *testing.T) {
	ws, name, sub := setupSubmoduleFixture(t)
	// Create untracked file inside submodule
	if err := os.WriteFile(filepath.Join(sub, "dirty.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	err := runSubmoduleSafetyChecks(ws, name, false)
	if err == nil || !strings.Contains(err.Error(), "dirty") {
		t.Fatalf("expected dirty error, got %v", err)
	}
	// --force should bypass
	if err := runSubmoduleSafetyChecks(ws, name, true); err != nil {
		t.Errorf("--force should bypass, got %v", err)
	}
}

func TestSafetyCheck_UnpushedCommits(t *testing.T) {
	ws, name, sub := setupSubmoduleFixture(t)
	// New local commit (not pushed to origin)
	if err := os.WriteFile(filepath.Join(sub, "x.txt"), []byte("y"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, sub, "add", ".")
	runGit(t, sub, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-m", "local only")
	err := runSubmoduleSafetyChecks(ws, name, false)
	if err == nil {
		t.Fatalf("expected error for unpushed commits")
	}
	if !strings.Contains(err.Error(), "unpushed") && !strings.Contains(err.Error(), "upstream") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSafetyCheck_LocalOnlyBranch(t *testing.T) {
	ws, name, sub := setupSubmoduleFixture(t)
	// Create a local-only branch with a commit
	runGit(t, sub, "checkout", "-b", "feature/local")
	if err := os.WriteFile(filepath.Join(sub, "x.txt"), []byte("y"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, sub, "add", ".")
	runGit(t, sub, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-m", "wip")
	err := runSubmoduleSafetyChecks(ws, name, false)
	if err == nil {
		t.Fatal("expected error for local-only branch")
	}
	if !strings.Contains(err.Error(), "upstream") && !strings.Contains(err.Error(), "local") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSafetyCheck_SHAMatchesOrigin(t *testing.T) {
	ws, name, sub := setupSubmoduleFixture(t)
	// Advance origin without updating the superproject pin
	urlOut, err := exec.Command("git", "-C", sub, "config", "--get", "remote.origin.url").Output()
	if err != nil {
		t.Fatalf("get url: %v", err)
	}
	originURL := strings.TrimPrefix(strings.TrimSpace(string(urlOut)), "file://")
	if err := os.WriteFile(filepath.Join(originURL, "new.txt"), []byte("z"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, originURL, "add", ".")
	runGit(t, originURL, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-m", "origin advance")
	// Fetch new SHA into submodule, but don't update superproject pin
	runGit(t, sub, "fetch")

	err = runSubmoduleSafetyChecks(ws, name, false)
	if err == nil {
		t.Fatal("expected SHA mismatch error")
	}
	if !strings.Contains(err.Error(), "pin") && !strings.Contains(err.Error(), "tip") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunMigrateProject_RejectsCloneProject(t *testing.T) {
	requireGit(t)
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".pylon", "config.yml"), []byte("version: \"0.1\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Plain clone: subdir is git-init'd, workspace is NOT a git repo
	projectDir := filepath.Join(workspace, "myproj")
	if out, err := exec.Command("git", "init", projectDir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}

	cmd := newMigrateProjectCmd()
	cmd.SetArgs([]string{"myproj"})

	oldWorkspace := flagWorkspace
	flagWorkspace = workspace
	defer func() { flagWorkspace = oldWorkspace }()

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for non-submodule project")
	}
	if !strings.Contains(err.Error(), "not a submodule") && !strings.Contains(err.Error(), "submodule") {
		t.Errorf("unexpected error: %v", err)
	}
}
