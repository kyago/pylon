package git

import (
	"os/exec"
	"testing"
)

// initTestRepo creates a temporary git repository with an initial commit.
func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	commands := [][]string{
		{"git", "init", dir},
		{"git", "-C", dir, "config", "user.email", "test@test.com"},
		{"git", "-C", dir, "config", "user.name", "Test"},
		{"git", "-C", dir, "commit", "--allow-empty", "-m", "init"},
	}
	for _, args := range commands {
		cmd := exec.Command(args[0], args[1:]...)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("setup %v failed: %v\n%s", args, err, output)
		}
	}
	return dir
}
