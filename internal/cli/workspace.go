package cli

import (
	"fmt"

	"github.com/kyago/pylon/internal/config"
)

// resolveRoot finds the pylon workspace root, starting from the --workspace
// flag value (or the current directory when unset). All commands share this
// helper so the "not a workspace" error message stays consistent.
func resolveRoot() (string, error) {
	startDir := flagWorkspace
	if startDir == "" {
		startDir = "."
	}
	root, err := config.FindWorkspaceRoot(startDir)
	if err != nil {
		return "", fmt.Errorf("pylon 워크스페이스가 아닙니다 — 'pylon init'을 먼저 실행하세요")
	}
	return root, nil
}
