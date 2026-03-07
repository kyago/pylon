package git

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
	"unicode"
)

// BranchName generates a branch name from the task spec.
// Format: {prefix}/{date}-{slug}
// Example: task/20260305-user-login
func BranchName(prefix, taskDesc string) string {
	date := time.Now().Format("20060102")
	slug := slugify(taskDesc)
	if len(slug) > 40 {
		slug = slug[:40]
	}
	return fmt.Sprintf("%s/%s-%s", prefix, date, slug)
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

// slugify converts a description into a URL-safe slug.
func slugify(s string) string {
	s = strings.ToLower(s)
	var result []rune
	prevDash := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			result = append(result, r)
			prevDash = false
		} else if !prevDash && len(result) > 0 {
			result = append(result, '-')
			prevDash = true
		}
	}
	return strings.TrimRight(string(result), "-")
}
