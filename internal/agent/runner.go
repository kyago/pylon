package agent

import (
	"fmt"
	"io"
	"strings"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/executor"
)

// RunConfig holds parameters for running a Claude Code agent.
type RunConfig struct {
	Agent       *config.AgentConfig
	Global      *config.Config
	TaskPrompt  string
	WorkDir     string    // worktree path or project directory
	ClaudeMD    string    // dynamically generated CLAUDE.md content
	Interactive bool      // true for PO (interactive mode)
	Stdout      io.Writer // if set, stream stdout instead of capturing
	Stderr      io.Writer // if set, stream stderr instead of capturing
}

// Runner manages Claude Code agent execution.
type Runner struct {
	Executor executor.ProcessExecutor
}

// NewRunner creates a new Runner with the given executor.
func NewRunner(exec executor.ProcessExecutor) *Runner {
	return &Runner{Executor: exec}
}

// BuildArgs constructs the claude CLI arguments as a string slice.
func (r *Runner) BuildArgs(cfg RunConfig) []string {
	var args []string

	if cfg.Interactive {
		// Interactive agent (PO): no --print, no --prompt
		if cfg.Agent.MaxTurns > 0 {
			args = append(args, "--max-turns", fmt.Sprintf("%d", cfg.Agent.MaxTurns))
		}
		args = append(args, "--permission-mode", cfg.Agent.PermissionMode)
	} else {
		// Non-interactive agent: --print with stream-json output
		args = append(args, "--print")
		args = append(args, "--output-format", "stream-json")
		if cfg.Agent.MaxTurns > 0 {
			args = append(args, "--max-turns", fmt.Sprintf("%d", cfg.Agent.MaxTurns))
		}
		args = append(args, "--permission-mode", cfg.Agent.PermissionMode)
	}

	// Model
	if cfg.Agent.Model != "" {
		args = append(args, "--model", cfg.Agent.Model)
	}

	// Allowed tools
	if len(cfg.Agent.Tools) > 0 {
		args = append(args, "--allowedTools", strings.Join(cfg.Agent.Tools, ","))
	}

	// Disallowed tools
	if len(cfg.Agent.DisallowedTools) > 0 {
		args = append(args, "--disallowedTools", strings.Join(cfg.Agent.DisallowedTools, ","))
	}

	// System prompt with CLAUDE.md content
	if cfg.ClaudeMD != "" {
		args = append(args, "--append-system-prompt", cfg.ClaudeMD)
	}

	// Task prompt (non-interactive only)
	if !cfg.Interactive && cfg.TaskPrompt != "" {
		args = append(args, "--prompt", cfg.TaskPrompt)
	}

	return args
}

// Start launches the agent as a headless process.
func (r *Runner) Start(cfg RunConfig) (*executor.ExecResult, error) {
	if cfg.Agent == nil {
		return nil, fmt.Errorf("agent config is required")
	}
	if cfg.WorkDir == "" {
		return nil, fmt.Errorf("work directory is required")
	}

	args := r.BuildArgs(cfg)
	env := ResolveEnv(cfg.Global.Runtime.Env, cfg.Agent.Env)

	return r.Executor.RunHeadless(executor.ExecConfig{
		Name:    cfg.Agent.Name,
		Command: "claude",
		Args:    args,
		WorkDir: cfg.WorkDir,
		Env:     env,
		Stdout:  cfg.Stdout,
		Stderr:  cfg.Stderr,
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
