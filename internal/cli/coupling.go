package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Coupling represents how a project is attached to the workspace.
type Coupling int

const (
	// CouplingClone means the project is a standalone git clone in the workspace.
	CouplingClone Coupling = iota
	// CouplingSubmodule means the project is registered as a git submodule of the workspace.
	CouplingSubmodule
)

func (c Coupling) String() string {
	switch c {
	case CouplingSubmodule:
		return "submodule"
	case CouplingClone:
		return "clone"
	default:
		return fmt.Sprintf("Coupling(%d)", int(c))
	}
}

// detectProjectCoupling determines how a project is attached to the workspace.
// Priority (per spec 003 §FR-5):
//  1. Workspace has no .git/ -> CouplingClone
//  2. Workspace .gitmodules has a [submodule] entry whose path equals projectName -> CouplingSubmodule
//  3. Otherwise -> CouplingClone
func detectProjectCoupling(workspaceRoot, projectName string) Coupling {
	if _, err := os.Stat(filepath.Join(workspaceRoot, ".git")); err != nil {
		return CouplingClone
	}
	data, err := os.ReadFile(filepath.Join(workspaceRoot, ".gitmodules"))
	if err != nil {
		return CouplingClone
	}
	if hasSubmodulePath(string(data), projectName) {
		return CouplingSubmodule
	}
	return CouplingClone
}

// hasSubmodulePath returns true if the .gitmodules contents declare a submodule
// whose path = projectName. Uses a simple line scanner that tolerates any
// whitespace around the key, =, and value.
func hasSubmodulePath(gitmodules, projectName string) bool {
	for _, line := range strings.Split(gitmodules, "\n") {
		trimmed := strings.TrimSpace(line)
		idx := strings.Index(trimmed, "=")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(trimmed[:idx])
		if key != "path" {
			continue
		}
		val := strings.TrimSpace(trimmed[idx+1:])
		if val == projectName {
			return true
		}
	}
	return false
}
