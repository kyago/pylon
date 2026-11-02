package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ProjectInfo holds basic information about a discovered project.
type ProjectInfo struct {
	Name string
	Path string
}

// FindWorkspaceRoot walks up from startDir looking for a .pylon/ directory
// that contains a config.yml file (indicating a workspace root, not a sub-project).
// Sub-project .pylon/ directories may exist without config.yml, so the search
// continues upward until a proper workspace root is found.
// Returns the workspace root path or an error if not found.
func FindWorkspaceRoot(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path: %w", err)
	}

	for {
		pylonDir := filepath.Join(dir, ".pylon")
		info, err := os.Stat(pylonDir)
		if err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("failed to stat %s: %w", pylonDir, err)
		}
		if err == nil && info.IsDir() {
			configPath := filepath.Join(pylonDir, "config.yml")
			configInfo, err := os.Stat(configPath)
			if err != nil && !os.IsNotExist(err) {
				return "", fmt.Errorf("failed to stat %s: %w", configPath, err)
			}
			if err == nil {
				if configInfo.Mode().IsRegular() {
					return dir, nil
				}
				return "", fmt.Errorf("invalid workspace config: %s exists but is not a regular file", configPath)
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root
			return "", fmt.Errorf("no .pylon/config.yml found (searched from %s to filesystem root)", startDir)
		}
		dir = parent
	}
}

// DiscoverProjects scans the workspace root for project directories.
// A project is identified as a subdirectory containing its own .pylon/ directory
// or being added as a standalone git clone in the workspace. Worktrees of a checkout
// in this workspace are not projects (see IsWorkspaceWorktree).
// Spec Reference: Section 4 "Workspace Structure"
func DiscoverProjects(root string) ([]ProjectInfo, error) {
	return scanPylonDirs(root, false)
}

// DiscoverWorktrees returns the subdirectories with a .pylon/ directory that
// DiscoverProjects leaves out as workspace worktrees.
func DiscoverWorktrees(root string) ([]ProjectInfo, error) {
	return scanPylonDirs(root, true)
}

func scanPylonDirs(root string, worktrees bool) ([]ProjectInfo, error) {
	var projects []ProjectInfo

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("failed to read workspace root: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		// Skip hidden directories and common non-project directories
		if strings.HasPrefix(name, ".") {
			continue
		}

		projectDir := filepath.Join(root, name)
		if IsWorkspaceWorktree(root, projectDir) != worktrees {
			continue
		}

		// Check if directory has its own .pylon/ (project-level config)
		projectPylonDir := filepath.Join(projectDir, ".pylon")
		if _, err := os.Stat(projectPylonDir); err == nil {
			projects = append(projects, ProjectInfo{
				Name: name,
				Path: projectDir,
			})
		}
	}

	return projects, nil
}

// IsWorkspaceWorktree reports whether dir is a linked git worktree whose main checkout
// is the workspace root or one of its direct subdirectories. Any other worktree (main
// checkout outside the workspace, bare or separate git dir) may be the only checkout
// here, so it stays a project. shortcut: a worktree whose main repository moved has no
// git dir to inspect and reports false, fix it with `git worktree repair`.
func IsWorkspaceWorktree(root, dir string) bool {
	// Resolved first: git follows symlinks, while filepath.Join cleans ".." textually.
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return false
	}
	data, err := os.ReadFile(filepath.Join(realDir, ".git"))
	if err != nil {
		return false
	}
	gitDir, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir: ")
	if !ok {
		return false
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(realDir, gitDir)
	}
	// Only linked worktrees have commondir; a submodule's git dir does not.
	data, err = os.ReadFile(filepath.Join(gitDir, "commondir"))
	if err != nil {
		return false
	}
	commonDir := strings.TrimSpace(string(data))
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(gitDir, commonDir)
	}
	commonDir, err = filepath.EvalSymlinks(commonDir)
	if err != nil || filepath.Base(commonDir) != ".git" {
		return false
	}
	mainDir := filepath.Dir(commonDir)
	return mainDir == realRoot || filepath.Dir(mainDir) == realRoot
}
