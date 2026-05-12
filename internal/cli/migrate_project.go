package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

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
	if err := os.RemoveAll(filepath.Join(workspaceRoot, ".git", "modules", projectName)); err != nil {
		return fmt.Errorf("remove cached modules: %w", err)
	}
	return nil
}
