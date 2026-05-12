package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDetectProjectCoupling_NoWorkspaceGit(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	if got := detectProjectCoupling(tmp, "sub"); got != CouplingClone {
		t.Errorf("got %v, want CouplingClone", got)
	}
}

func TestDetectProjectCoupling_WorkspaceGitNoGitmodules(t *testing.T) {
	requireGit(t)
	tmp := t.TempDir()
	if out, err := exec.Command("git", "init", tmp).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if err := os.MkdirAll(filepath.Join(tmp, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	if got := detectProjectCoupling(tmp, "sub"); got != CouplingClone {
		t.Errorf("got %v, want CouplingClone", got)
	}
}

func TestDetectProjectCoupling_Submodule(t *testing.T) {
	requireGit(t)
	tmp := t.TempDir()
	if out, err := exec.Command("git", "init", tmp).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	gitmodules := "[submodule \"sub\"]\n\tpath = sub\n\turl = https://example.com/sub.git\n"
	if err := os.WriteFile(filepath.Join(tmp, ".gitmodules"), []byte(gitmodules), 0644); err != nil {
		t.Fatal(err)
	}
	if got := detectProjectCoupling(tmp, "sub"); got != CouplingSubmodule {
		t.Errorf("got %v, want CouplingSubmodule", got)
	}
}
