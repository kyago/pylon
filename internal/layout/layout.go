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

// RuntimeDir returns the pipeline runtime state directory (.pylon/runtime).
func RuntimeDir(root string) string {
	return filepath.Join(PylonDir(root), "runtime")
}

// MemoryDir returns the markdown memory store root (.pylon/memory).
func MemoryDir(root string) string {
	return filepath.Join(PylonDir(root), "memory")
}

// ProjectMemoryDir returns one project's memory directory (.pylon/memory/<project>).
func ProjectMemoryDir(root, project string) string {
	return filepath.Join(MemoryDir(root), project)
}

// HistoryDir returns the file-based work history root (.pylon/history).
func HistoryDir(root string) string {
	return filepath.Join(PylonDir(root), "history")
}

// LearningDir returns the approval-gated learning workflow root.
func LearningDir(root string) string {
	return filepath.Join(PylonDir(root), "learning")
}

// LearningCandidatesDir returns the curator candidate queue directory.
func LearningCandidatesDir(root string) string {
	return filepath.Join(LearningDir(root), "candidates")
}

// CommandsDir returns the pipeline command directory (.pylon/commands).
func CommandsDir(root string) string {
	return filepath.Join(PylonDir(root), "commands")
}

// VerifyConfigPath returns the verification config path (.pylon/verify.yml) for a
// workspace root or a project directory.
func VerifyConfigPath(dir string) string {
	return filepath.Join(PylonDir(dir), "verify.yml")
}

// ScriptsDir returns the pipeline bash script directory (.pylon/scripts/bash).
func ScriptsDir(root string) string {
	return filepath.Join(PylonDir(root), "scripts", "bash")
}

// CodexSkillsDir returns the repo-scoped codex skill discovery directory
// (.agents/skills) under the workspace root. Codex walks ancestor directories
// probing for this path and loads every SKILL.md inside as a workflow.
func CodexSkillsDir(root string) string {
	return filepath.Join(root, ".agents", "skills")
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

// RootClaudePath returns the workspace-root CLAUDE.md (a thin @AGENTS.md import marker).
func RootClaudePath(root string) string {
	return filepath.Join(root, "CLAUDE.md")
}

// RootAgentsPath returns the workspace-root AGENTS.md (AI-authored operating guide).
func RootAgentsPath(root string) string {
	return filepath.Join(root, "AGENTS.md")
}
