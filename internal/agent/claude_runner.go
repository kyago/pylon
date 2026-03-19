package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kyago/pylon/internal/executor"
)

// ClaudeCodeRunner executes agents via the Claude Code CLI.
// It implements the AgentRunner interface.
type ClaudeCodeRunner struct {
	Executor executor.ProcessExecutor
}

// NewClaudeCodeRunner creates a new ClaudeCodeRunner with the given executor.
func NewClaudeCodeRunner(exec executor.ProcessExecutor) *ClaudeCodeRunner {
	return &ClaudeCodeRunner{Executor: exec}
}

// Start launches the agent as a headless process using the Claude CLI.
func (r *ClaudeCodeRunner) Start(cfg RunConfig) (*executor.ExecResult, error) {
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
	injectPylonEnv(env, cfg)

	ctx := cfg.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := resolveTimeout(cfg.Agent.Timeout, cfg.Global.Runtime.TaskTimeout)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return r.Executor.RunHeadless(executor.ExecConfig{
		Name:    cfg.Agent.Name,
		Command: "claude",
		Args:    args,
		WorkDir: cfg.WorkDir,
		Env:     env,
		Ctx:     ctx,
		Stdout:  cfg.Stdout,
		Stderr:  cfg.Stderr,
	})
}

// BuildArgs constructs the claude CLI arguments as a string slice.
func (r *ClaudeCodeRunner) BuildArgs(cfg RunConfig) []string {
	var args []string

	if cfg.Interactive {
		if cfg.Agent.MaxTurns > 0 {
			args = append(args, "--max-turns", fmt.Sprintf("%d", cfg.Agent.MaxTurns))
		}
		args = append(args, "--permission-mode", cfg.Agent.PermissionMode)
	} else {
		args = append(args, "--print")
		args = append(args, "--verbose")
		args = append(args, "--output-format", "stream-json")
		if cfg.Agent.MaxTurns > 0 {
			args = append(args, "--max-turns", fmt.Sprintf("%d", cfg.Agent.MaxTurns))
		}
		args = append(args, "--permission-mode", cfg.Agent.PermissionMode)
	}

	if cfg.Agent.Model != "" {
		args = append(args, "--model", cfg.Agent.Model)
	}

	if len(cfg.Agent.Tools) > 0 {
		args = append(args, "--allowedTools", strings.Join(cfg.Agent.Tools, ","))
	}

	if len(cfg.Agent.DisallowedTools) > 0 {
		args = append(args, "--disallowedTools", strings.Join(cfg.Agent.DisallowedTools, ","))
	}

	if cfg.ClaudeMD != "" {
		args = append(args, "--append-system-prompt", cfg.ClaudeMD)
	}

	if !cfg.Interactive && cfg.TaskPrompt != "" {
		args = append(args, cfg.TaskPrompt)
	}

	return args
}

// resolveTimeout determines the effective timeout for an agent execution.
func resolveTimeout(agentTimeout string, globalTimeout string) time.Duration {
	if agentTimeout != "" {
		if d, err := time.ParseDuration(agentTimeout); err == nil && d > 0 {
			return d
		}
	}
	if globalTimeout != "" {
		if d, err := time.ParseDuration(globalTimeout); err == nil && d > 0 {
			return d
		}
	}
	return 30 * time.Minute
}

// injectPylonEnv adds pylon-specific environment variables to the env map.
func injectPylonEnv(env map[string]string, cfg RunConfig) {
	if cfg.Agent != nil {
		env["PYLON_AGENT_NAME"] = cfg.Agent.Name
	}
	if cfg.PipelineID != "" {
		env["PYLON_PIPELINE_ID"] = cfg.PipelineID
	}
	if cfg.TaskID != "" {
		env["PYLON_TASK_ID"] = cfg.TaskID
	}
}

// Verify interface compliance at compile time.
var _ AgentRunner = (*ClaudeCodeRunner)(nil)

// Backward compatibility: Runner is an alias for ClaudeCodeRunner.
type Runner = ClaudeCodeRunner

// NewRunner creates a new Runner (alias for NewClaudeCodeRunner).
func NewRunner(exec executor.ProcessExecutor) *Runner {
	return NewClaudeCodeRunner(exec)
}

