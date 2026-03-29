package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kyago/pylon/internal/config"
)

func TestCheckExcludeStatus(t *testing.T) {
	requireGit(t)
	tmpDir := t.TempDir()
	cmd := exec.Command("git", "init", tmpDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}

	// Before adding entry: git repo, no entry
	isGit, hasEntry := checkExcludeStatus(tmpDir)
	if !isGit {
		t.Error("expected isGitRepo=true")
	}
	if hasEntry {
		t.Error("expected hasEntry=false before adding entry")
	}

	// Add .pylon/ to exclude
	if err := excludePylonFromSubmodule(tmpDir); err != nil {
		t.Fatalf("excludePylonFromSubmodule failed: %v", err)
	}

	// After adding entry: git repo, has entry
	isGit, hasEntry = checkExcludeStatus(tmpDir)
	if !isGit {
		t.Error("expected isGitRepo=true")
	}
	if !hasEntry {
		t.Error("expected hasEntry=true after adding entry")
	}
}

func TestCheckExcludeStatus_NotGitRepo(t *testing.T) {
	tmpDir := t.TempDir()
	isGit, hasEntry := checkExcludeStatus(tmpDir)
	if isGit {
		t.Error("expected isGitRepo=false for non-git directory")
	}
	if hasEntry {
		t.Error("expected hasEntry=false for non-git directory")
	}
}

func TestCheckExcludeStatus_NoExcludeFile(t *testing.T) {
	requireGit(t)
	tmpDir := t.TempDir()
	cmd := exec.Command("git", "init", tmpDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}

	// Remove the exclude file if it exists
	excludePath := filepath.Join(tmpDir, ".git", "info", "exclude")
	os.Remove(excludePath)

	isGit, hasEntry := checkExcludeStatus(tmpDir)
	if !isGit {
		t.Error("expected isGitRepo=true")
	}
	if hasEntry {
		t.Error("expected hasEntry=false when exclude file is missing")
	}
}

func TestCheckExcludeStatus_WithOtherEntries(t *testing.T) {
	requireGit(t)
	tmpDir := t.TempDir()
	cmd := exec.Command("git", "init", tmpDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}

	// Write exclude file with other entries but NOT .pylon/
	excludePath := filepath.Join(tmpDir, ".git", "info", "exclude")
	content := strings.Join([]string{
		"# git ls-files --others --exclude-from=.git/info/exclude",
		"*.log",
		"node_modules/",
		"",
	}, "\n")
	if err := os.WriteFile(excludePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	isGit, hasEntry := checkExcludeStatus(tmpDir)
	if !isGit {
		t.Error("expected isGitRepo=true")
	}
	if hasEntry {
		t.Error("expected hasEntry=false when .pylon/ is not in exclude")
	}

	// Now add .pylon/ and verify
	if err := excludePylonFromSubmodule(tmpDir); err != nil {
		t.Fatal(err)
	}
	isGit, hasEntry = checkExcludeStatus(tmpDir)
	if !isGit || !hasEntry {
		t.Error("expected isGitRepo=true, hasEntry=true after adding .pylon/")
	}
}

func TestCheckSubmoduleExcludes_Fix(t *testing.T) {
	requireGit(t)

	// Set up a workspace with a project that has .pylon/ but no exclude entry
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".pylon", "config.yml"), []byte("version: \"0.1\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	projectDir := filepath.Join(tmpDir, "myproject")
	cmd := exec.Command("git", "init", projectDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}

	// Verify project is discoverable (explicit precondition)
	projects, err := config.DiscoverProjects(tmpDir)
	if err != nil {
		t.Fatalf("DiscoverProjects failed: %v", err)
	}
	if len(projects) == 0 {
		t.Fatal("precondition failed: no projects discovered")
	}

	// Remove exclude file to ensure .pylon/ is NOT excluded
	excludePath := filepath.Join(projectDir, ".git", "info", "exclude")
	os.Remove(excludePath)

	// Verify .pylon/ is not excluded
	_, hasEntry := checkExcludeStatus(projectDir)
	if hasEntry {
		t.Fatal("precondition failed: .pylon/ should not be in exclude yet")
	}

	// Run checkSubmoduleExcludes with fix=true
	oldWorkspace := flagWorkspace
	flagWorkspace = tmpDir
	defer func() { flagWorkspace = oldWorkspace }()

	ok := checkSubmoduleExcludes(true)
	if !ok {
		t.Error("expected checkSubmoduleExcludes to return true after successful fix")
	}

	// Verify .pylon/ is now excluded
	_, hasEntry = checkExcludeStatus(projectDir)
	if !hasEntry {
		t.Error("expected .pylon/ to be in exclude after fix")
	}
}

func TestCheckSubmoduleExcludes_DetectMissing(t *testing.T) {
	requireGit(t)

	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".pylon", "config.yml"), []byte("version: \"0.1\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	projectDir := filepath.Join(tmpDir, "myproject")
	cmd := exec.Command("git", "init", projectDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}

	excludePath := filepath.Join(projectDir, ".git", "info", "exclude")
	os.Remove(excludePath)

	oldWorkspace := flagWorkspace
	flagWorkspace = tmpDir
	defer func() { flagWorkspace = oldWorkspace }()

	// fix=false should report missing and return false
	ok := checkSubmoduleExcludes(false)
	if ok {
		t.Error("expected checkSubmoduleExcludes to return false when excludes are missing")
	}

	// Should still NOT be excluded (fix=false)
	_, hasEntry := checkExcludeStatus(projectDir)
	if hasEntry {
		t.Error("expected .pylon/ to remain missing when fix=false")
	}
}

func TestCheckSubmoduleExcludes_AllOK(t *testing.T) {
	requireGit(t)

	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".pylon", "config.yml"), []byte("version: \"0.1\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	projectDir := filepath.Join(tmpDir, "myproject")
	cmd := exec.Command("git", "init", projectDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}

	// Pre-seed the exclude entry
	if err := excludePylonFromSubmodule(projectDir); err != nil {
		t.Fatal(err)
	}

	oldWorkspace := flagWorkspace
	flagWorkspace = tmpDir
	defer func() { flagWorkspace = oldWorkspace }()

	ok := checkSubmoduleExcludes(false)
	if !ok {
		t.Error("expected checkSubmoduleExcludes to return true when all projects have excludes")
	}

	// Verify entry still intact
	_, hasEntry := checkExcludeStatus(projectDir)
	if !hasEntry {
		t.Error("expected .pylon/ to remain in exclude")
	}
}

func TestCheckSubmoduleExcludes_SkipsNonGitProjects(t *testing.T) {
	requireGit(t)

	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".pylon", "config.yml"), []byte("version: \"0.1\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a non-git project with .pylon/
	projectDir := filepath.Join(tmpDir, "non-git-project")
	if err := os.MkdirAll(filepath.Join(projectDir, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}

	oldWorkspace := flagWorkspace
	flagWorkspace = tmpDir
	defer func() { flagWorkspace = oldWorkspace }()

	// Should return true (non-git projects are skipped, not flagged as missing)
	ok := checkSubmoduleExcludes(false)
	if !ok {
		t.Error("expected checkSubmoduleExcludes to return true when only non-git projects exist")
	}
}

func TestNewDoctorCmd_FixExcludesFlag(t *testing.T) {
	cmd := newDoctorCmd()

	f := cmd.Flags().Lookup("fix-excludes")
	if f == nil {
		t.Fatal("--fix-excludes flag not found")
	}
	if f.DefValue != "false" {
		t.Errorf("--fix-excludes default = %q, want %q", f.DefValue, "false")
	}
}

func TestResolveGitExcludePath(t *testing.T) {
	requireGit(t)
	tmpDir := t.TempDir()
	cmd := exec.Command("git", "init", tmpDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}

	path, err := resolveGitExcludePath(tmpDir)
	if err != nil {
		t.Fatalf("resolveGitExcludePath failed: %v", err)
	}
	if !strings.HasSuffix(path, filepath.Join("info", "exclude")) {
		t.Errorf("expected path ending with info/exclude, got %s", path)
	}
}

func TestResolveGitExcludePath_NotGitRepo(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := resolveGitExcludePath(tmpDir)
	if err == nil {
		t.Fatal("expected error for non-git directory, got nil")
	}
	if !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("unexpected error: %v", err)
	}
}
