// Package git provides Git utility functions for pylon.
// Spec Reference: Section 8 "Git Worktree Isolation"
package git

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WorktreeManager manages Git worktrees for agent isolation.
type WorktreeManager struct {
	Enabled     bool
	AutoCleanup bool
	Runner      CommandRunner // nil이면 defaultRunner 사용
}

// runner returns the effective CommandRunner for this manager.
func (w *WorktreeManager) runner() CommandRunner {
	if w.Runner != nil {
		return w.Runner
	}
	return defaultRunner
}

// NewWorktreeManager creates a WorktreeManager from config.
func NewWorktreeManager(enabled, autoCleanup bool) *WorktreeManager {
	return &WorktreeManager{
		Enabled:     enabled,
		AutoCleanup: autoCleanup,
	}
}

// Create creates a new Git worktree for an agent.
// Path: {projectDir}/.git/pylon-worktrees/{agentName}-{taskSlug}
func (w *WorktreeManager) Create(projectDir, agentName, taskBranch string) (string, error) {
	if !w.Enabled {
		return projectDir, nil // no isolation, use project dir directly
	}

	worktreeBase := filepath.Join(projectDir, ".git", "pylon-worktrees")
	if err := os.MkdirAll(worktreeBase, 0755); err != nil {
		return "", fmt.Errorf("failed to create worktree base: %w", err)
	}

	slug := SanitizeBranchName(agentName)
	worktreePath := filepath.Join(worktreeBase, slug)

	// Each agent gets its own branch: {taskBranch}/{agentName}
	agentBranch := fmt.Sprintf("%s/%s", taskBranch, SanitizeBranchName(agentName))

	// Create worktree with new branch
	output, err := w.runner().Run(projectDir, "git", "worktree", "add", "-b", agentBranch, worktreePath)
	if err != nil {
		return "", fmt.Errorf("failed to create worktree: %w\n%s", err, output)
	}

	return worktreePath, nil
}

// Remove removes a Git worktree.
func (w *WorktreeManager) Remove(projectDir, worktreePath string) error {
	output, err := w.runner().Run(projectDir, "git", "worktree", "remove", worktreePath, "--force")
	if err != nil {
		return fmt.Errorf("failed to remove worktree: %w\n%s", err, output)
	}
	return nil
}

// Cleanup removes all pylon worktrees from a project.
func (w *WorktreeManager) Cleanup(projectDir string) error {
	worktreeBase := filepath.Join(projectDir, ".git", "pylon-worktrees")
	if _, err := os.Stat(worktreeBase); os.IsNotExist(err) {
		return nil
	}

	entries, err := os.ReadDir(worktreeBase)
	if err != nil {
		return fmt.Errorf("failed to read worktree dir: %w", err)
	}

	var errs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(worktreeBase, entry.Name())
		if err := w.Remove(projectDir, path); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", entry.Name(), err))
		}
	}

	// Also prune stale worktree entries
	w.runner().Run(projectDir, "git", "worktree", "prune") // best-effort

	if len(errs) > 0 {
		return fmt.Errorf("some worktrees failed to clean:\n%s", strings.Join(errs, "\n"))
	}
	return nil
}

// MergeBranch merges an agent branch into the current branch (task branch).
func (w *WorktreeManager) MergeBranch(projectDir, agentBranch string) error {
	output, err := w.runner().Run(projectDir,
		"git", "merge", "--no-ff", agentBranch,
		"-m", fmt.Sprintf("merge: %s 에이전트 작업 통합", agentBranch))
	if err != nil {
		return fmt.Errorf("merge failed for %s: %w\n%s", agentBranch, err, output)
	}
	return nil
}

// DeleteBranch deletes a local branch after successful merge.
func (w *WorktreeManager) DeleteBranch(projectDir, branchName string) error {
	output, err := w.runner().Run(projectDir, "git", "branch", "-d", branchName)
	if err != nil {
		return fmt.Errorf("failed to delete branch %s: %w\n%s", branchName, err, output)
	}
	return nil
}

// HeadSHA returns the current HEAD commit SHA in the given directory.
func (w *WorktreeManager) HeadSHA(projectDir string) (string, error) {
	output, err := w.runner().Run(projectDir, "git", "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("failed to get HEAD SHA: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// ResetHard resets the working tree to the given commit SHA.
func (w *WorktreeManager) ResetHard(projectDir, commitSHA string) error {
	output, err := w.runner().Run(projectDir, "git", "reset", "--hard", commitSHA)
	if err != nil {
		return fmt.Errorf("git reset --hard %s failed: %w\n%s", commitSHA, err, output)
	}
	return nil
}

// AbortMerge aborts an in-progress merge.
func (w *WorktreeManager) AbortMerge(projectDir string) error {
	_, err := w.runner().Run(projectDir, "git", "merge", "--abort")
	return err
}

// SanitizeBranchName makes a string safe for use as a branch/directory name.
func SanitizeBranchName(name string) string {
	replacer := strings.NewReplacer("/", "-", " ", "-", ":", "-")
	return replacer.Replace(name)
}
