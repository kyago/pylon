package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunInit_DoesNotGitInitWorkspace(t *testing.T) {
	requireGit(t)
	tmp := t.TempDir()
	oldWorkspace := flagWorkspace
	flagWorkspace = tmp
	defer func() { flagWorkspace = oldWorkspace }()

	cmd := newInitCmd()
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, ".git")); err == nil {
		t.Errorf("expected workspace to NOT be a git repo, but .git/ exists")
	}
}
