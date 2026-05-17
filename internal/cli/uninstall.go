package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kyago/pylon/internal/config"
)

func newUninstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Completely remove pylon from workspace",
		Long: `Completely remove pylon from the current workspace.

This removes:
  1. Runtime artifacts (.claude/, CLAUDE.md)
  2. Project-level .pylon/ directories
  3. Workspace .pylon/ directory (config, domain, agents, database)
  4. Pylon entries from .gitignore

Project source code is preserved by default.

Use --remove-projects to also remove project clone directories.
Use --remove-binary to also delete the pylon binary from $GOPATH/bin.`,
		RunE: runUninstall,
	}

	cmd.Flags().BoolP("force", "f", false, "skip confirmation prompt")
	cmd.Flags().Bool("dry-run", false, "show what would be removed without removing")
	cmd.Flags().Bool("remove-projects", false, "also remove project clone directories")
	cmd.Flags().Bool("remove-binary", false, "also remove the pylon binary from $GOPATH/bin")

	return cmd
}

// uninstallPlan holds the list of actions to perform during uninstall.
type uninstallPlan struct {
	runtimeFiles   []string // .claude/, CLAUDE.md
	projectPylons  []string // {project}/.pylon/ directories
	cloneProjects  []string // standalone clone projects (only if --remove-projects)
	workspacePylon string   // .pylon/ directory
	gitignorePath  string   // .gitignore to clean
	binaryPath     string   // pylon binary path (only if --remove-binary)
}

func runUninstall(cmd *cobra.Command, args []string) error {
	force, _ := cmd.Flags().GetBool("force")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	removeProjects, _ := cmd.Flags().GetBool("remove-projects")
	removeBinary, _ := cmd.Flags().GetBool("remove-binary")

	// Find workspace
	startDir := flagWorkspace
	if startDir == "" {
		startDir = "."
	}
	root, err := config.FindWorkspaceRoot(startDir)
	if err != nil {
		return fmt.Errorf("not in a pylon workspace: %w", err)
	}

	// Build uninstall plan
	plan, err := buildUninstallPlan(root, removeProjects, removeBinary)
	if err != nil {
		return fmt.Errorf("failed to build uninstall plan: %w", err)
	}

	// Show plan
	printUninstallPlan(plan)

	if dryRun {
		fmt.Println("\n(dry-run mode: no changes were made)")
		return nil
	}

	// Confirm
	if !force {
		fmt.Print("\nAre you sure? (y/N): ")
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			fmt.Println("Uninstall cancelled.")
			return nil
		}
	}

	// Execute uninstall
	return executeUninstall(root, plan)
}

func buildUninstallPlan(root string, removeProjects, removeBinary bool) (*uninstallPlan, error) {
	plan := &uninstallPlan{}

	// 1. Runtime artifacts
	claudeDir := filepath.Join(root, ".claude")
	if dirExists(claudeDir) {
		plan.runtimeFiles = append(plan.runtimeFiles, claudeDir)
	}
	claudeMD := filepath.Join(root, "CLAUDE.md")
	if fileExists(claudeMD) {
		plan.runtimeFiles = append(plan.runtimeFiles, claudeMD)
	}

	// 2. Discover projects and their .pylon/ directories
	projects, err := config.DiscoverProjects(root)
	if err != nil {
		return nil, fmt.Errorf("failed to discover projects: %w", err)
	}
	for _, p := range projects {
		projectPylon := filepath.Join(p.Path, ".pylon")
		if dirExists(projectPylon) {
			plan.projectPylons = append(plan.projectPylons, projectPylon)
		}
		if removeProjects {
			plan.cloneProjects = append(plan.cloneProjects, p.Name)
		}
	}

	// 3. Workspace .pylon/
	pylonDir := filepath.Join(root, ".pylon")
	if dirExists(pylonDir) {
		plan.workspacePylon = pylonDir
	}

	// 4. .gitignore
	gitignorePath := filepath.Join(root, ".gitignore")
	if fileExists(gitignorePath) {
		plan.gitignorePath = gitignorePath
	}

	// 5. Binary
	if removeBinary {
		binaryPath, err := findPylonBinary()
		if err == nil && binaryPath != "" {
			plan.binaryPath = binaryPath
		}
	}

	return plan, nil
}

func printUninstallPlan(plan *uninstallPlan) {
	fmt.Println("The following items will be removed:")
	fmt.Println()

	if len(plan.runtimeFiles) > 0 {
		fmt.Println("  [Runtime artifacts]")
		for _, f := range plan.runtimeFiles {
			fmt.Printf("    - %s\n", f)
		}
	}

	if len(plan.projectPylons) > 0 {
		fmt.Println("  [Project .pylon/ directories]")
		for _, p := range plan.projectPylons {
			fmt.Printf("    - %s\n", p)
		}
	}

	if len(plan.cloneProjects) > 0 {
		fmt.Println("  [Clone project directories]")
		for _, c := range plan.cloneProjects {
			fmt.Printf("    - %s/\n", c)
		}
	}

	if plan.workspacePylon != "" {
		fmt.Println("  [Workspace]")
		fmt.Printf("    - %s\n", plan.workspacePylon)
	}

	if plan.gitignorePath != "" {
		fmt.Println("  [.gitignore cleanup]")
		fmt.Printf("    - %s (remove pylon-related entries)\n", plan.gitignorePath)
	}

	if plan.binaryPath != "" {
		fmt.Println("  [Binary]")
		fmt.Printf("    - %s\n", plan.binaryPath)
	}
}

func executeUninstall(root string, plan *uninstallPlan) error {
	var errors []string

	// Step 1: Remove runtime artifacts
	for _, f := range plan.runtimeFiles {
		if err := os.RemoveAll(f); err != nil {
			errors = append(errors, fmt.Sprintf("failed to remove runtime artifact (%s): %v", f, err))
		} else {
			fmt.Printf("✓ Removed: %s\n", f)
		}
	}

	// Step 2: Remove project-level .pylon/ directories
	for _, p := range plan.projectPylons {
		if err := os.RemoveAll(p); err != nil {
			errors = append(errors, fmt.Sprintf("failed to remove project .pylon/ (%s): %v", p, err))
		} else {
			fmt.Printf("✓ Removed: %s\n", p)
		}
	}

	// Step 3: Remove clone project directories (if requested)
	if len(plan.cloneProjects) > 0 {
		for _, name := range plan.cloneProjects {
			projDir := filepath.Join(root, name)
			if err := os.RemoveAll(projDir); err != nil {
				errors = append(errors, fmt.Sprintf("failed to remove clone project (%s): %v", name, err))
			} else {
				fmt.Printf("✓ Removed clone project: %s\n", name)
			}
		}
	}

	// Step 4: Remove workspace .pylon/
	if plan.workspacePylon != "" {
		if err := os.RemoveAll(plan.workspacePylon); err != nil {
			errors = append(errors, fmt.Sprintf("failed to remove workspace .pylon/: %v", err))
		} else {
			fmt.Printf("✓ Removed: %s\n", plan.workspacePylon)
		}
	}

	// Step 5: Clean .gitignore
	if plan.gitignorePath != "" {
		if err := cleanGitignoreFull(plan.gitignorePath); err != nil {
			errors = append(errors, fmt.Sprintf("failed to clean .gitignore: %v", err))
		} else {
			fmt.Printf("✓ Cleaned .gitignore\n")
		}
	}

	// Step 6: Remove binary
	if plan.binaryPath != "" {
		if err := os.Remove(plan.binaryPath); err != nil {
			errors = append(errors, fmt.Sprintf("failed to remove binary (%s): %v", plan.binaryPath, err))
		} else {
			fmt.Printf("✓ Removed binary: %s\n", plan.binaryPath)
		}
	}

	fmt.Println()
	if len(errors) > 0 {
		fmt.Println("Some items could not be removed:")
		for _, e := range errors {
			fmt.Printf("  ⚠ %s\n", e)
		}
		return fmt.Errorf("failed to remove %d item(s)", len(errors))
	}

	fmt.Println("Pylon has been completely removed.")
	return nil
}

// cleanGitignoreFull removes all pylon-related entries from .gitignore,
// including both workspace and Claude Code generated entries.
func cleanGitignoreFull(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	perm := info.Mode().Perm()

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	var cleaned []string
	pylonSection := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect pylon section markers (both workspace and generated)
		if trimmed == "# pylon" || trimmed == "# Pylon" ||
			trimmed == "# Pylon runtime (agent communication, state)" ||
			trimmed == "# Pylon-generated Claude Code config (dynamically generated)" {
			pylonSection = true
			continue
		}

		if pylonSection {
			// Skip pylon-specific entries
			if trimmed == "" {
				pylonSection = false
				continue
			}
			if strings.HasPrefix(trimmed, ".pylon/") ||
				strings.HasPrefix(trimmed, ".claude/") ||
				trimmed == "CLAUDE.md" {
				continue
			}
			// Non-pylon entry ends the section
			pylonSection = false
		}

		cleaned = append(cleaned, line)
	}

	// Remove trailing empty lines that may result from section removal
	for len(cleaned) > 0 && strings.TrimSpace(cleaned[len(cleaned)-1]) == "" {
		cleaned = cleaned[:len(cleaned)-1]
	}
	if len(cleaned) > 0 {
		cleaned = append(cleaned, "") // ensure final newline
	}

	return os.WriteFile(path, []byte(strings.Join(cleaned, "\n")), perm)
}

// findPylonBinary locates the pylon binary in $GOPATH/bin.
func findPylonBinary() (string, error) {
	// Try GOPATH/bin first
	goPathCmd := exec.Command("go", "env", "GOPATH")
	output, err := goPathCmd.Output()
	if err == nil {
		goPath := strings.TrimSpace(string(output))
		if goPath != "" {
			binaryPath := filepath.Join(goPath, "bin", "pylon")
			if fileExists(binaryPath) {
				return binaryPath, nil
			}
		}
	}

	// Fallback: check if pylon is in PATH
	path, err := exec.LookPath("pylon")
	if err != nil {
		return "", fmt.Errorf("pylon binary not found")
	}
	fmt.Printf("  Warning: pylon binary found at %s (not in $GOPATH/bin, may be system-installed)\n", path)
	return path, nil
}
