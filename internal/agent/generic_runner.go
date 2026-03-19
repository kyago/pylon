package agent

import (
	"context"
	"fmt"

	"github.com/kyago/pylon/internal/executor"
)

// GenericCLIRunner executes agents via an arbitrary CLI command.
// This enables non-Claude backends (e.g., local LLMs, custom scripts).
type GenericCLIRunner struct {
	Executor executor.ProcessExecutor
	Command  string // the CLI binary to execute (e.g., "ollama", "my-agent")
}

// NewGenericCLIRunner creates a new GenericCLIRunner for the given command.
func NewGenericCLIRunner(exec executor.ProcessExecutor, command string) *GenericCLIRunner {
	return &GenericCLIRunner{Executor: exec, Command: command}
}

// Start launches the agent using the configured CLI command.
// The task prompt is passed as the first positional argument.
// Agent-specific and global environment variables are merged and injected.
func (r *GenericCLIRunner) Start(cfg RunConfig) (*executor.ExecResult, error) {
	if cfg.Agent == nil {
		return nil, fmt.Errorf("agent config is required")
	}
	if cfg.WorkDir == "" {
		return nil, fmt.Errorf("work directory is required")
	}

	var globalEnv map[string]string
	if cfg.Global != nil {
		globalEnv = cfg.Global.Runtime.Env
	}
	env := ResolveEnv(globalEnv, cfg.Agent.Env)
	injectPylonEnv(env, cfg)

	// Build args: pass task prompt and optional model
	var args []string
	if cfg.Agent.Model != "" {
		args = append(args, "--model", cfg.Agent.Model)
	}
	if cfg.TaskPrompt != "" {
		args = append(args, cfg.TaskPrompt)
	}

	ctx := cfg.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	var globalTimeout string
	if cfg.Global != nil {
		globalTimeout = cfg.Global.Runtime.TaskTimeout
	}
	timeout := resolveTimeout(cfg.Agent.Timeout, globalTimeout)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return r.Executor.RunHeadless(executor.ExecConfig{
		Name:    cfg.Agent.Name,
		Command: r.Command,
		Args:    args,
		WorkDir: cfg.WorkDir,
		Env:     env,
		Ctx:     ctx,
		Stdout:  cfg.Stdout,
		Stderr:  cfg.Stderr,
	})
}

// Verify interface compliance at compile time.
var _ AgentRunner = (*GenericCLIRunner)(nil)
