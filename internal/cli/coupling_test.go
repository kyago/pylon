package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestHasSubmodulePath(t *testing.T) {
	cases := []struct {
		name       string
		gitmodules string
		project    string
		want       bool
	}{
		{"empty", "", "sub", false},
		{"exact match", "[submodule \"sub\"]\n\tpath = sub\n\turl = x\n", "sub", true},
		{"non-matching path", "[submodule \"bar\"]\n\tpath = bar\n", "foo", false},
		{"prefix collision", "[submodule \"subproject\"]\n\tpath = subproject\n", "sub", false},
		{"nested path", "[submodule \"sub\"]\n\tpath = projects/sub\n", "sub", false},
		{"pathological key only", "[submodule \"sub\"]\n\tpathological = sub\n", "sub", false},
		{"path with leading space and equals tight", "[submodule \"sub\"]\n   path=sub\n", "sub", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasSubmodulePath(tc.gitmodules, tc.project); got != tc.want {
				t.Errorf("hasSubmodulePath(%q) = %v, want %v", tc.gitmodules, got, tc.want)
			}
		})
	}
}

func TestDetectProjectCoupling_NonMatchingGitmodules(t *testing.T) {
	requireGit(t)
	tmp := t.TempDir()
	if out, err := exec.Command("git", "init", tmp).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if err := os.WriteFile(filepath.Join(tmp, ".gitmodules"),
		[]byte("[submodule \"bar\"]\n\tpath = bar\n\turl = https://example.com/bar.git\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := detectProjectCoupling(tmp, "foo"); got != CouplingClone {
		t.Errorf("got %v, want CouplingClone", got)
	}
}

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
