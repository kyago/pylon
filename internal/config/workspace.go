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
// or being added as a standalone git clone in the workspace. Linked git worktrees
// are checkouts of a repository rather than projects, so they are skipped.
// Spec Reference: Section 4 "Workspace Structure"
func DiscoverProjects(root string) ([]ProjectInfo, error) {
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
		if isLinkedWorktree(projectDir) {
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

// isLinkedWorktree reports whether dir is a linked git worktree: its .git is a gitlink
// file whose git dir has a commondir file (a submodule's git dir has none). A worktree
// whose main repository has moved has no git dir to inspect and is reported as false.
func isLinkedWorktree(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, ".git"))
	if err != nil {
		return false
	}
	gitDir, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir: ")
	if !ok {
		return false
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(dir, gitDir)
	}
	_, err = os.Stat(filepath.Join(gitDir, "commondir"))
	return err == nil
}
