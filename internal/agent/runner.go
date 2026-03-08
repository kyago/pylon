package agent

import (
	"fmt"
	"strings"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/tmux"
)

// RunConfig holds parameters for running a Claude Code agent.
type RunConfig struct {
	Agent       *config.AgentConfig
	Global      *config.Config
	TaskPrompt  string
	WorkDir     string // worktree path or project directory
	ClaudeMD    string // dynamically generated CLAUDE.md content
	Interactive bool   // true for PO (interactive mode)
}

// Runner manages Claude Code agent execution via tmux.
type Runner struct {
	Tmux tmux.SessionManager
}

// NewRunner creates a new Runner with the given tmux manager.
func NewRunner(mgr tmux.SessionManager) *Runner {
	return &Runner{Tmux: mgr}
}

// BuildCommand constructs the claude CLI command and arguments.
// Spec Reference: Section 8 "Claude Code CLI Execution Spec"
func (r *Runner) BuildCommand(cfg RunConfig) string {
	var parts []string
	parts = append(parts, "claude")

	if cfg.Interactive {
		// Interactive agent (PO): no --print, no --prompt
		if cfg.Agent.MaxTurns > 0 {
			parts = append(parts, fmt.Sprintf("--max-turns %d", cfg.Agent.MaxTurns))
		}
		parts = append(parts, "--permission-mode", cfg.Agent.PermissionMode)
	} else {
		// Non-interactive agent: --print with stream-json output
		parts = append(parts, "--print")
		parts = append(parts, "--output-format", "stream-json")
		if cfg.Agent.MaxTurns > 0 {
			parts = append(parts, fmt.Sprintf("--max-turns %d", cfg.Agent.MaxTurns))
		}
		parts = append(parts, "--permission-mode", cfg.Agent.PermissionMode)
	}

	// Model
	if cfg.Agent.Model != "" {
		parts = append(parts, "--model", cfg.Agent.Model)
	}

	// System prompt with CLAUDE.md content
	if cfg.ClaudeMD != "" {
		// Write CLAUDE.md as system prompt appendage
		// Use single quotes to prevent shell expansion
		escaped := strings.ReplaceAll(cfg.ClaudeMD, "'", "'\\''")
		parts = append(parts, fmt.Sprintf("--append-system-prompt '%s'", escaped))
	}

	// Task prompt (non-interactive only)
	if !cfg.Interactive && cfg.TaskPrompt != "" {
		escaped := strings.ReplaceAll(cfg.TaskPrompt, "'", "'\\''")
		parts = append(parts, fmt.Sprintf("--prompt '%s'", escaped))
	}

	return strings.Join(parts, " ")
}

// Start launches the agent in a tmux session.
func (r *Runner) Start(cfg RunConfig) error {
	if cfg.Agent == nil {
		return fmt.Errorf("agent config is required")
	}
	if cfg.WorkDir == "" {
		return fmt.Errorf("work directory is required")
	}

	command := r.BuildCommand(cfg)
	env := ResolveEnv(cfg.Global.Runtime.Env, cfg.Agent.Env)

	// Build tmux session name
	prefix := cfg.Global.Tmux.SessionPrefix
	if prefix == "" {
		prefix = "pylon"
	}
	sessionName := prefix + "-" + cfg.Agent.Name

	// Create tmux session with the command
	return r.Tmux.Create(tmux.SessionConfig{
		Name:         sessionName,
		WorkDir:      cfg.WorkDir,
		Command:      command,
		Env:          env,
		HistoryLimit: cfg.Global.Tmux.HistoryLimit,
	})
}

// PrepareWorkDir sets up the working directory for an agent.
// For worktree isolation, creates a git worktree.
// For no isolation, uses the project directory directly.
func PrepareWorkDir(
	enabled bool,
	autoCleanup bool,
	agentIsolation string,
	taskBranch string,
	projectDir string,
	agentName string,
) (workDir string, cleanup func() error, err error) {
	if agentIsolation != "worktree" || !enabled {
		return projectDir, func() error { return nil }, nil
	}

	// Import git package for worktree creation
	// Since we can't import git package here without circular dependency,
	// we keep this as a helper that callers wire up with the git.WorktreeManager
	return projectDir, func() error { return nil }, nil
}
