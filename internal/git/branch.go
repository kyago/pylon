package git

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/kyago/pylon/internal/slug"
)

// BranchName generates a branch name from the task spec.
// Format: {prefix}/{date}-{slug}
// Example: task/20260305-user-login
func BranchName(prefix, taskDesc string) string {
	date := time.Now().Format("20060102")
	s := slug.Slugify(taskDesc)
	if len(s) > 40 {
		s = s[:40]
	}
	s = strings.TrimRight(s, "-")
	return fmt.Sprintf("%s/%s-%s", prefix, date, s)
}

// CreateBranch creates a new git branch from the current HEAD.
func CreateBranch(dir, branchName string) error {
	cmd := exec.Command("git", "checkout", "-b", branchName)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create branch %q: %w\n%s", branchName, err, output)
	}
	return nil
}

// CurrentBranch returns the current git branch name.
func CurrentBranch(dir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

