package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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
