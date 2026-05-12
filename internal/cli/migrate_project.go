package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/kyago/pylon/internal/config"
)

func newMigrateProjectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate-project [name]",
		Short: "Convert a legacy git submodule project to a standalone clone",
		Long: `Convert a project that was previously added as a git submodule
into a standalone git clone in the workspace.

Safety checks (spec 003 §5.1) block migration when:
  - the submodule working tree is dirty
  - there are local commits not pushed to origin
  - there are local-only branches
  - the pinned SHA differs from origin's default branch tip

Use --dry-run to preview check results without changing any state.
Use --force to override safety checks (data loss possible).`,
		Args: cobra.ExactArgs(1),
		RunE: runMigrateProject,
	}
	cmd.Flags().Bool("dry-run", false, "report safety check results without changing state")
	cmd.Flags().Bool("force", false, "override safety checks (may discard local changes)")
	return cmd
}

func runMigrateProject(cmd *cobra.Command, args []string) error {
	projectName := args[0]

	startDir := flagWorkspace
	if startDir == "" {
		startDir = "."
	}
	root, err := config.FindWorkspaceRoot(startDir)
	if err != nil {
		return fmt.Errorf("not in a pylon workspace — run 'pylon init' first")
	}

	if err := validateProjectName(projectName); err != nil {
		return err
	}

	if detectProjectCoupling(root, projectName) != CouplingSubmodule {
		return fmt.Errorf("%s is not a submodule (nothing to migrate)", projectName)
	}

	dryRun, _ := cmd.Flags().GetBool("dry-run")
	force, _ := cmd.Flags().GetBool("force")

	if err := runSubmoduleSafetyChecks(root, projectName, force); err != nil {
		return fmt.Errorf("migration blocked: %w", err)
	}
	if dryRun {
		fmt.Println("✓ All safety checks passed (dry-run; no changes were made)")
		return nil
	}

	return performMigration(root, projectName)
}

// performMigration is implemented in Task 10.
func performMigration(workspaceRoot, projectName string) error {
	_ = workspaceRoot
	_ = projectName
	return fmt.Errorf("not implemented (Task 10)")
}

// runSubmoduleSafetyChecks performs spec 003 §5.1 safety checks against a
// legacy submodule before migrating it. Phase 5 (Task 9) replaces this stub
// with the full check suite.
func runSubmoduleSafetyChecks(workspaceRoot, projectName string, force bool) error {
	_ = workspaceRoot
	_ = projectName
	_ = force
	return nil
}

// teardownSubmodule deregisters a submodule from the workspace and cleans
// cached git data. Used by `add-project --force --migrate` and by
// `pylon migrate-project` (Phase 5).
func teardownSubmodule(workspaceRoot, projectName string) error {
	deregCmd := exec.Command("git", "submodule", "deinit", "-f", projectName)
	deregCmd.Dir = workspaceRoot
	if out, err := deregCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("submodule deinit: %w\n%s", err, out)
	}
	rmCmd := exec.Command("git", "rm", "-f", projectName)
	rmCmd.Dir = workspaceRoot
	if out, err := rmCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git rm: %w\n%s", err, out)
	}
	// os.RemoveAll is a no-op if the path does not exist.
	if err := os.RemoveAll(filepath.Join(workspaceRoot, ".git", "modules", projectName)); err != nil {
		return fmt.Errorf("remove cached modules: %w", err)
	}
	return nil
}
