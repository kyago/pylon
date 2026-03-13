package agent

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/executor"
	"github.com/kyago/pylon/internal/git"
)

// RunConfig holds parameters for running a Claude Code agent.
type RunConfig struct {
	Ctx         context.Context   // if set, cancels agent process on context cancellation
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
	if cfg.Global == nil {
		return nil, fmt.Errorf("global config is required")
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
		Ctx:     cfg.Ctx,
		Stdout:  cfg.Stdout,
		Stderr:  cfg.Stderr,
	})
}

// BuildTaskPrompt generates a short task instruction for a headless agent.
// The prompt directs the agent to read its inbox for detailed task information.
func BuildTaskPrompt(role, agentName, taskID, inboxDir string) string {
	return fmt.Sprintf(
		"당신은 %s입니다.\ninbox 파일을 읽고 태스크를 수행하세요.\ninbox: %s/%s/%s.task.json\n완료 후 outbox에 결과를 작성하세요.",
		role, inboxDir, agentName, taskID,
	)
}

// PrepareWorkDir sets up the working directory for an agent.
// When a WorktreeManager is provided and isolation is "worktree", creates a git worktree.
// Otherwise, uses the project directory directly.
func PrepareWorkDir(
	wm *git.WorktreeManager,
	agentIsolation string,
	taskBranch string,
	projectDir string,
	agentName string,
) (workDir string, cleanup func() error, err error) {
	noop := func() error { return nil }

	if wm == nil || agentIsolation != "worktree" || !wm.Enabled {
		return projectDir, noop, nil
	}

	wtPath, err := wm.Create(projectDir, agentName, taskBranch)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create worktree for %s: %w", agentName, err)
	}

	cleanup = func() error {
		if wm.AutoCleanup {
			return wm.Remove(wtPath)
		}
		return nil
	}

	return wtPath, cleanup, nil
}
