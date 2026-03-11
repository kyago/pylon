package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/kyago/pylon/internal/config"
)

func newDestroyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:        "destroy",
		Short:      "Remove pylon workspace (deprecated: use 'pylon uninstall')",
		Long: `Remove the .pylon/ directory and clean up all pylon-related resources.
Git submodules are preserved.

DEPRECATED: Use 'pylon uninstall' for comprehensive cleanup including
runtime artifacts, project-level configs, and optional binary removal.

Spec Reference: Section 7 "pylon destroy"`,
		Deprecated: "use 'pylon uninstall' instead for comprehensive cleanup",
		RunE:       runDestroy,
	}

	cmd.Flags().BoolP("force", "f", false, "skip confirmation prompt")

	return cmd
}

func runDestroy(cmd *cobra.Command, args []string) error {
	force, _ := cmd.Flags().GetBool("force")

	// Find workspace
	startDir := flagWorkspace
	if startDir == "" {
		startDir = "."
	}
	root, err := config.FindWorkspaceRoot(startDir)
	if err != nil {
		return fmt.Errorf("not in a pylon workspace: %w", err)
	}

	pylonDir := filepath.Join(root, ".pylon")

	// Confirm unless --force
	if !force {
		fmt.Printf("This will:\n")
		fmt.Printf("  1. Remove %s/\n", pylonDir)
		fmt.Printf("  2. Clean .gitignore of pylon entries\n")
		fmt.Print("\nAre you sure? (y/N): ")

		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			fmt.Println("Destroy cancelled.")
			return nil
		}
	}

	// Step 1: Remove .pylon/ directory
	if err := os.RemoveAll(pylonDir); err != nil {
		return fmt.Errorf("failed to remove %s: %w", pylonDir, err)
	}
	fmt.Printf("Removed %s/\n", pylonDir)

	// Step 2: Clean .gitignore
	gitignorePath := filepath.Join(root, ".gitignore")
	if err := cleanGitignore(gitignorePath); err != nil {
		// Non-fatal: warn but don't fail
		fmt.Printf("Warning: could not clean .gitignore: %v\n", err)
	} else {
		fmt.Println("Cleaned .gitignore of pylon entries.")
	}

	fmt.Println("\nPylon workspace destroyed.")
	return nil
}

// cleanGitignore removes pylon-related entries from .gitignore.
// Delegates to cleanGitignoreFull which handles all pylon section markers.
func cleanGitignore(path string) error {
	return cleanGitignoreFull(path)
}
