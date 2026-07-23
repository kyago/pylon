package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func validateProjectName(name string) error {
	if name == "" {
		return fmt.Errorf("project name cannot be empty")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("invalid project name: %q", name)
	}
	if strings.ContainsAny(name, `/\`) || filepath.IsAbs(name) {
		return fmt.Errorf("project name must not contain path separators: %q", name)
	}
	if filepath.Clean(name) != name {
		return fmt.Errorf("invalid project name: %q", name)
	}
	return nil
}

// inferProjectName extracts project name from git URL.
func inferProjectName(repoURL string) string {
	// Handle "https://github.com/user/repo.git" or "git@github.com:user/repo.git"
	name := repoURL
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	if idx := strings.LastIndex(name, ":"); idx >= 0 {
		name = name[idx+1:]
	}
	name = strings.TrimSuffix(name, ".git")
	return name
}

// resolveGitExcludePath returns the absolute path to a project's .git/info/exclude file.
// It uses "git rev-parse --git-dir" to correctly resolve non-standard git layouts
// (e.g. worktrees, or legacy submodule clones whose .git is a gitlink file).
func resolveGitExcludePath(projectDir string) (string, error) {
	gitDirCmd := exec.Command("git", "rev-parse", "--git-dir")
	gitDirCmd.Dir = projectDir
	out, err := gitDirCmd.Output()
	if err != nil {
		return "", fmt.Errorf("not a git repository: %w", err)
	}

	gitDir := strings.TrimSpace(string(out))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(projectDir, gitDir)
	}

	return filepath.Join(gitDir, "info", "exclude"), nil
}

// excludePylonFromRepo adds ".pylon/" to the repo's .git/info/exclude. Works for both submodules and standalone clones.
func excludePylonFromRepo(projectDir string) error {
	excludePath, err := resolveGitExcludePath(projectDir)
	if err != nil {
		return err
	}

	// Read existing exclude file if it exists
	existing, err := os.ReadFile(excludePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read exclude file: %w", err)
	}
	for _, line := range strings.Split(string(existing), "\n") {
		if strings.TrimSpace(line) == ".pylon/" {
			return nil // already excluded
		}
	}

	// Ensure info/ directory exists
	infoDir := filepath.Dir(excludePath)
	if err := os.MkdirAll(infoDir, 0755); err != nil {
		return fmt.Errorf("failed to create git info directory: %w", err)
	}

	f, err := os.OpenFile(excludePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open exclude file: %w", err)
	}
	defer f.Close()

	// Add newline before entry if file doesn't end with one
	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}
	if _, err := f.WriteString(".pylon/\n"); err != nil {
		return err
	}

	return nil
}

// techStack holds detected technology information.
