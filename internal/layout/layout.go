// Package layout centralizes the well-known paths inside a pylon workspace.
// All ".pylon" / ".claude" path construction should go through these helpers
// so the directory layout is defined in exactly one place.
package layout

import "path/filepath"

// PylonDir returns the .pylon directory under the given workspace root.
func PylonDir(root string) string {
	return filepath.Join(root, ".pylon")
}

// ConfigPath returns the workspace config file path (.pylon/config.yml).
func ConfigPath(root string) string {
	return filepath.Join(PylonDir(root), "config.yml")
}

// DBPath returns the SQLite store path (.pylon/pylon.db).
func DBPath(root string) string {
	return filepath.Join(PylonDir(root), "pylon.db")
}

// RuntimeDir returns the pipeline runtime state directory (.pylon/runtime).
func RuntimeDir(root string) string {
	return filepath.Join(PylonDir(root), "runtime")
}

// CommandsDir returns the pipeline command directory (.pylon/commands).
func CommandsDir(root string) string {
	return filepath.Join(PylonDir(root), "commands")
}

// ScriptsDir returns the pipeline bash script directory (.pylon/scripts/bash).
func ScriptsDir(root string) string {
	return filepath.Join(PylonDir(root), "scripts", "bash")
}

// ClaudeDir returns the Claude CLI directory under the workspace root (.claude).
func ClaudeDir(root string) string {
	return filepath.Join(root, ".claude")
}

// ClaudeAgentsDir returns the Claude agent discovery directory (.claude/agents).
func ClaudeAgentsDir(root string) string {
	return filepath.Join(ClaudeDir(root), "agents")
}

// ClaudeCommandsDir returns the Claude slash command directory (.claude/commands).
func ClaudeCommandsDir(root string) string {
	return filepath.Join(ClaudeDir(root), "commands")
}

// AgentLinkTarget returns the relative symlink target used inside
// .claude/agents/ to point at .pylon/agents/<name>.
func AgentLinkTarget(name string) string {
	return filepath.Join("..", "..", ".pylon", "agents", name)
}
