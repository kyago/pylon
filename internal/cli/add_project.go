package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/layout"
)

func newAddProjectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add-project [git-url]",
		Short: "Add a project as a standalone git clone in the workspace",
		Long: `Add a project to the workspace as a standalone git clone.

Analyzes the codebase and creates project-level .pylon/ configuration
including context.md and default agent definitions.

If the project directory already exists, use --force to re-clone or
--skip-clone to keep the existing directory and only generate .pylon/ config.

Spec Reference: spec 003.`,
		Args: cobra.ExactArgs(1),
		RunE: runAddProject,
	}

	cmd.Flags().String("name", "", "project directory name (default: inferred from URL)")
	cmd.Flags().Bool("force", false, "remove existing directory and re-clone")
	cmd.Flags().Bool("skip-clone", false, "skip git clone; use existing directory for .pylon/ setup only")

	return cmd
}

func runAddProject(cmd *cobra.Command, args []string) error {
	repoURL := args[0]

	// Find workspace root
	root, err := resolveRoot()
	if err != nil {
		return err
	}

	// Determine project name
	projectName, _ := cmd.Flags().GetString("name")
	if projectName == "" {
		projectName = inferProjectName(repoURL)
	}

	force, _ := cmd.Flags().GetBool("force")
	skipClone, _ := cmd.Flags().GetBool("skip-clone")

	if force && skipClone {
		return fmt.Errorf("--force and --skip-clone are mutually exclusive")
	}

	// Validate project name to prevent path traversal
	if err := validateProjectName(projectName); err != nil {
		return err
	}

	// Before prepareProjectDirectory: --force would delete the worktree.
	if config.IsWorkspaceWorktree(root, filepath.Join(root, projectName)) {
		return fmt.Errorf("%s is a git worktree of a checkout in this workspace, not a project", projectName)
	}

	projectDir, err := prepareProjectDirectory(root, projectName, force, skipClone)
	if err != nil {
		return err
	}
	if !skipClone {
		if err := cloneProject(root, repoURL, projectName); err != nil {
			return err
		}
	}
	contextPath, err := scaffoldProject(projectDir, projectName)
	if err != nil {
		return err
	}

	fmt.Println()
	fmt.Printf("Project %s added successfully.\n", projectName)
	refreshProjectList(root)

	// Exclude .pylon/ from project git tracking via .git/info/exclude
	if err := excludePylonFromRepo(projectDir); err != nil {
		fmt.Printf("⚠ Could not auto-exclude .pylon/ from git tracking: %v\n", err)
		fmt.Println("  Manually add '.pylon/' to the project's .git/info/exclude")
	} else {
		fmt.Println("✓ .pylon/ excluded from project git tracking")
	}

	fmt.Println()
	fmt.Print(projectNextSteps(contextPath))

	return nil
}

func prepareProjectDirectory(root, projectName string, force, skipClone bool) (string, error) {
	projectDir := filepath.Join(root, projectName)
	info, statErr := os.Stat(projectDir)
	switch {
	case statErr == nil && !info.IsDir():
		return "", fmt.Errorf("%s exists but is not a directory", projectName)
	case statErr == nil && force:
		fmt.Printf("Removing existing directory: %s\n", projectName)
		if err := os.RemoveAll(projectDir); err != nil {
			return "", fmt.Errorf("failed to remove existing directory: %w", err)
		}
	case statErr == nil && skipClone:
		fmt.Printf("Using existing directory: %s\n", projectName)
	case statErr == nil:
		return "", fmt.Errorf("directory %s already exists (use --force to re-clone or --skip-clone to use existing)", projectName)
	case !os.IsNotExist(statErr):
		return "", fmt.Errorf("cannot access %s: %w", projectName, statErr)
	case skipClone:
		return "", fmt.Errorf("directory %s does not exist; cannot use --skip-clone", projectName)
	}
	return projectDir, nil
}

func cloneProject(root, repoURL, projectName string) error {
	fmt.Printf("Cloning: %s\n", repoURL)
	gitCmd := exec.Command("git", "clone", repoURL, projectName)
	gitCmd.Dir = root
	if output, err := gitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to clone: %w\n%s", err, output)
	}
	fmt.Printf("✓ Cloned: %s\n", projectName)
	return nil
}

func scaffoldProject(projectDir, projectName string) (string, error) {
	pylonDir := layout.PylonDir(projectDir)
	if err := os.MkdirAll(pylonDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create project .pylon/: %w", err)
	}
	fmt.Println("Analyzing codebase...")
	stack := detectTechStack(projectDir)
	contextPath := filepath.Join(pylonDir, "context.md")
	if err := os.WriteFile(contextPath, []byte(generateContextMD(projectName, stack)), 0o644); err != nil {
		return "", fmt.Errorf("failed to create context.md: %w", err)
	}
	fmt.Println("✓ context.md generated")
	return contextPath, nil
}

func projectNextSteps(contextPath string) string {
	return fmt.Sprintf(`Next steps:
  1. Review: %s
  2. Start working: open the workspace in Claude Code, Codex, or Paseo
`, contextPath)
}

// validateProjectName ensures the project name is a safe single directory name
// without path separators or traversal components.
