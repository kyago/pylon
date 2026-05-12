package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
// legacy submodule before migrating it. If force is true, all checks are bypassed.
func runSubmoduleSafetyChecks(workspaceRoot, projectName string, force bool) error {
	if force {
		return nil
	}
	subDir := filepath.Join(workspaceRoot, projectName)
	if err := checkWorkingTreeClean(subDir); err != nil {
		return err
	}
	if err := checkAllCommitsPushed(subDir); err != nil {
		return err
	}
	if err := checkSHAMatchesOrigin(workspaceRoot, projectName, subDir); err != nil {
		return err
	}
	return nil
}

// checkWorkingTreeClean returns an error if the submodule has any untracked,
// modified, or staged files.
func checkWorkingTreeClean(subDir string) error {
	out, err := exec.Command("git", "-C", subDir, "status", "--porcelain").Output()
	if err != nil {
		return fmt.Errorf("git status failed: %w", err)
	}
	if len(out) > 0 {
		return fmt.Errorf("working tree is dirty (untracked or modified files); commit or stash, or rerun with --force to discard")
	}
	return nil
}

// checkAllCommitsPushed verifies every local branch has an upstream and is
// not ahead of it. Detached HEAD repositories fall back to checking HEAD
// against origin/HEAD.
func checkAllCommitsPushed(subDir string) error {
	out, err := exec.Command("git", "-C", subDir, "for-each-ref", "--format=%(refname:short)", "refs/heads/").Output()
	if err != nil {
		return fmt.Errorf("for-each-ref failed: %w", err)
	}
	branches := strings.Fields(strings.TrimSpace(string(out)))
	if len(branches) == 0 {
		return checkHEADAgainstOrigin(subDir)
	}
	for _, b := range branches {
		upstream, err := exec.Command("git", "-C", subDir, "rev-parse", "--abbrev-ref", b+"@{upstream}").Output()
		if err != nil || strings.TrimSpace(string(upstream)) == "" {
			return fmt.Errorf("branch %q has no upstream (potentially unpushed commits)", b)
		}
		ahead, err := exec.Command("git", "-C", subDir, "rev-list", "--count", b+"@{upstream}.."+b).Output()
		if err != nil {
			return fmt.Errorf("rev-list failed on %q: %w", b, err)
		}
		if strings.TrimSpace(string(ahead)) != "0" {
			return fmt.Errorf("branch %q has unpushed commits (%s ahead of upstream)", b, strings.TrimSpace(string(ahead)))
		}
	}
	return nil
}

// checkHEADAgainstOrigin compares HEAD to origin/HEAD for detached HEAD repos.
func checkHEADAgainstOrigin(subDir string) error {
	headSHA, err := exec.Command("git", "-C", subDir, "rev-parse", "HEAD").Output()
	if err != nil {
		return nil // empty repo
	}
	originHead, err := exec.Command("git", "-C", subDir, "rev-parse", "origin/HEAD").Output()
	if err != nil {
		return nil // origin/HEAD not set; SHA mismatch check (§5.1 #4) will handle
	}
	if strings.TrimSpace(string(headSHA)) != strings.TrimSpace(string(originHead)) {
		return fmt.Errorf("HEAD is not at origin's default branch tip (detached or diverged)")
	}
	return nil
}

// checkSHAMatchesOrigin verifies that the SHA the superproject has pinned
// for this submodule matches origin's default branch tip. Returns nil if
// they match or if either side cannot be resolved (e.g. origin/HEAD unset).
func checkSHAMatchesOrigin(workspaceRoot, projectName, subDir string) error {
	pinOut, err := exec.Command("git", "-C", workspaceRoot, "ls-tree", "HEAD", projectName).Output()
	if err != nil {
		return nil
	}
	parts := strings.Fields(string(pinOut))
	if len(parts) < 3 {
		return nil
	}
	pinned := parts[2]

	var originSHA []byte
	if out, err := exec.Command("git", "-C", subDir, "rev-parse", "origin/HEAD").Output(); err == nil {
		originSHA = out
	} else {
		for _, ref := range []string{"origin/main", "origin/master"} {
			if out, err := exec.Command("git", "-C", subDir, "rev-parse", ref).Output(); err == nil {
				originSHA = out
				break
			}
		}
	}
	if len(originSHA) == 0 {
		return nil
	}
	tip := strings.TrimSpace(string(originSHA))
	if pinned != tip {
		return fmt.Errorf("pinned SHA %s differs from origin default branch tip %s (pin will be lost; rerun with --force to proceed)", pinned[:8], tip[:8])
	}
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
