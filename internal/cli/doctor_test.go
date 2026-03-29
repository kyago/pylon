package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestHasExcludeEntry(t *testing.T) {
	requireGit(t)
	tmpDir := t.TempDir()
	cmd := exec.Command("git", "init", tmpDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}

	// Before adding entry, should return false
	if hasExcludeEntry(tmpDir) {
		t.Error("expected hasExcludeEntry to return false before adding entry")
	}

	// Add .pylon/ to exclude
	if err := excludePylonFromSubmodule(tmpDir); err != nil {
		t.Fatalf("excludePylonFromSubmodule failed: %v", err)
	}

	// After adding entry, should return true
	if !hasExcludeEntry(tmpDir) {
		t.Error("expected hasExcludeEntry to return true after adding entry")
	}
}

func TestHasExcludeEntry_NotGitRepo(t *testing.T) {
	tmpDir := t.TempDir()
	// Should return false for non-git directory (not panic/error)
	if hasExcludeEntry(tmpDir) {
		t.Error("expected hasExcludeEntry to return false for non-git directory")
	}
}

func TestHasExcludeEntry_NoExcludeFile(t *testing.T) {
	requireGit(t)
	tmpDir := t.TempDir()
	cmd := exec.Command("git", "init", tmpDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}

	// Remove the exclude file if it exists
	excludePath := filepath.Join(tmpDir, ".git", "info", "exclude")
	os.Remove(excludePath)

	if hasExcludeEntry(tmpDir) {
		t.Error("expected hasExcludeEntry to return false when exclude file is missing")
	}
}

func TestCheckSubmoduleExcludes_Fix(t *testing.T) {
	requireGit(t)

	// Set up a workspace with a project that has .pylon/ but no exclude entry
	tmpDir := t.TempDir()

	// Init workspace
	if err := os.MkdirAll(filepath.Join(tmpDir, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".pylon", "config.yml"), []byte("version: \"0.1\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a project subdirectory with .pylon/ and git init
	projectDir := filepath.Join(tmpDir, "myproject")
	cmd := exec.Command("git", "init", projectDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}

	// Remove exclude file to ensure .pylon/ is NOT excluded
	excludePath := filepath.Join(projectDir, ".git", "info", "exclude")
	os.Remove(excludePath)

	// Verify .pylon/ is not excluded
	if hasExcludeEntry(projectDir) {
		t.Fatal("precondition failed: .pylon/ should not be in exclude yet")
	}

	// Run checkSubmoduleExcludes with fix=true
	oldWorkspace := flagWorkspace
	flagWorkspace = tmpDir
	defer func() { flagWorkspace = oldWorkspace }()

	checkSubmoduleExcludes(true)

	// Verify .pylon/ is now excluded
	if !hasExcludeEntry(projectDir) {
		t.Error("expected .pylon/ to be in exclude after fix")
	}
}

func TestCheckSubmoduleExcludes_DetectMissing(t *testing.T) {
	requireGit(t)

	// Set up a workspace with a project missing exclude
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

	// Remove exclude file
	excludePath := filepath.Join(projectDir, ".git", "info", "exclude")
	os.Remove(excludePath)

	// Run checkSubmoduleExcludes with fix=false (detect only)
	oldWorkspace := flagWorkspace
	flagWorkspace = tmpDir
	defer func() { flagWorkspace = oldWorkspace }()

	// Capture that it doesn't fix (just reports)
	checkSubmoduleExcludes(false)

	// Should still NOT be excluded (fix=false)
	if hasExcludeEntry(projectDir) {
		t.Error("expected .pylon/ to remain missing when fix=false")
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

func TestCheckSubmoduleExcludes_AllOK(t *testing.T) {
	requireGit(t)

	// Set up a workspace where all projects already have exclude entries
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

	// Should report all OK without errors (fix=false)
	checkSubmoduleExcludes(false)

	// Verify entry still intact
	if !hasExcludeEntry(projectDir) {
		t.Error("expected .pylon/ to remain in exclude")
	}
}

func TestHasExcludeEntry_WithOtherEntries(t *testing.T) {
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

	if hasExcludeEntry(tmpDir) {
		t.Error("expected hasExcludeEntry to return false when .pylon/ is not in exclude")
	}

	// Now add .pylon/ and verify
	if err := excludePylonFromSubmodule(tmpDir); err != nil {
		t.Fatal(err)
	}
	if !hasExcludeEntry(tmpDir) {
		t.Error("expected hasExcludeEntry to return true after adding .pylon/")
	}
}
