package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunInit_DoesNotGitInitWorkspace(t *testing.T) {
	requireGit(t)
	installFakeFossil(t)
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

func installFakeFossil(t *testing.T) {
	t.Helper()
	binDir := t.TempDir()
	script := `#!/bin/sh
case "$1" in
  version)
    echo "This is fossil version 2.28 [test]"
    ;;
  init)
    : > "$2"
    ;;
  open)
    shift
    while [ "$#" -gt 0 ]; do
      if [ "$1" = "--workdir" ]; then
        mkdir -p "$2"
        : > "$2/.fslckout"
        exit 0
      fi
      shift
    done
    ;;
esac
`
	path := filepath.Join(binDir, "fossil")
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
