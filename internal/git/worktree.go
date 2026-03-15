// Package git provides Git utility functions for pylon.
// Spec Reference: Section 8 "Git Worktree Isolation"
package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// WorktreeManager manages Git worktrees for agent isolation.
type WorktreeManager struct {
	Enabled     bool
	AutoCleanup bool
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

	slug := sanitizeBranchName(agentName)
	worktreePath := filepath.Join(worktreeBase, slug)

	// Each agent gets its own branch: {taskBranch}/{agentName}
	agentBranch := fmt.Sprintf("%s/%s", taskBranch, sanitizeBranchName(agentName))

	// Create worktree with new branch
	cmd := exec.Command("git", "worktree", "add", "-b", agentBranch, worktreePath)
	cmd.Dir = projectDir
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to create worktree: %w\n%s", err, output)
	}

	return worktreePath, nil
}

// Remove removes a Git worktree.
func (w *WorktreeManager) Remove(projectDir, worktreePath string) error {
	cmd := exec.Command("git", "worktree", "remove", worktreePath, "--force")
	cmd.Dir = projectDir
	if output, err := cmd.CombinedOutput(); err != nil {
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
	pruneCmd := exec.Command("git", "worktree", "prune")
	pruneCmd.Dir = projectDir
	pruneCmd.CombinedOutput() // best-effort

	if len(errs) > 0 {
		return fmt.Errorf("some worktrees failed to clean:\n%s", strings.Join(errs, "\n"))
	}
	return nil
}

// sanitizeBranchName makes a string safe for use as a branch/directory name.
func sanitizeBranchName(name string) string {
	replacer := strings.NewReplacer("/", "-", " ", "-", ":", "-")
	return replacer.Replace(name)
}
