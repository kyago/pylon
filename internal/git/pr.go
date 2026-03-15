package git

import (
	"fmt"
	"strings"
)

// PRCreateConfig holds parameters for creating a pull request.
// Spec Reference: Section 13 "Git Branch Strategy", config.yml git.pr
type PRCreateConfig struct {
	Title     string
	Body      string
	Branch    string
	Base      string   // config.yml git.default_base
	Reviewers []string // config.yml git.pr.reviewers
	Draft     bool     // config.yml git.pr.draft
}

// CreatePR creates a GitHub pull request using the gh CLI.
func CreatePR(projectDir string, cfg PRCreateConfig) (string, error) {
	return createPRWith(defaultRunner, projectDir, cfg)
}

// createPRWith is the internal implementation that accepts a CommandRunner.
func createPRWith(runner CommandRunner, projectDir string, cfg PRCreateConfig) (string, error) {
	args := []string{"pr", "create"}

	if cfg.Title != "" {
		args = append(args, "--title", cfg.Title)
	}
	if cfg.Body != "" {
		args = append(args, "--body", cfg.Body)
	}
	if cfg.Base != "" {
		args = append(args, "--base", cfg.Base)
	}
	if cfg.Draft {
		args = append(args, "--draft")
	}
	for _, r := range cfg.Reviewers {
		args = append(args, "--reviewer", r)
	}

	output, err := runner.Run(projectDir, "gh", args...)
	if err != nil {
		return "", fmt.Errorf("failed to create PR: %w\n%s", err, output)
	}

	prURL := strings.TrimSpace(string(output))
	return prURL, nil
}

// PushBranch pushes the current branch to remote.
func PushBranch(projectDir, branch string) error {
	return pushBranchWith(defaultRunner, projectDir, branch)
}

// pushBranchWith is the internal implementation that accepts a CommandRunner.
func pushBranchWith(runner CommandRunner, projectDir, branch string) error {
	output, err := runner.Run(projectDir, "git", "push", "-u", "origin", branch)
	if err != nil {
		return fmt.Errorf("failed to push branch: %w\n%s", err, output)
	}
	return nil
}
