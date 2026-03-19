package agent

import (
	"context"
	"fmt"
	"io"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/executor"
	"github.com/kyago/pylon/internal/git"
)

// AgentRunner is the interface for executing agents.
// Implementations must respect the ctx in RunConfig for cancellation and timeout.
// (ADR-003: ctx-based timeout is the interface contract)
type AgentRunner interface {
	Start(cfg RunConfig) (*executor.ExecResult, error)
}

// RunConfig holds parameters for running an agent.
type RunConfig struct {
	Ctx         context.Context
	Agent       *config.AgentConfig
	Global      *config.Config
	TaskPrompt  string
	WorkDir     string
	ClaudeMD    string
	Interactive bool
	Stdout      io.Writer
	Stderr      io.Writer
	PipelineID  string // injected as PYLON_PIPELINE_ID
	TaskID      string // injected as PYLON_TASK_ID
}

// NewRunnerForBackend returns the appropriate AgentRunner for the given backend.
// "claude" or "" returns a ClaudeCodeRunner, anything else returns a GenericCLIRunner.
func NewRunnerForBackend(backend string, exec executor.ProcessExecutor) AgentRunner {
	switch backend {
	case "", "claude", "claude-code":
		return NewClaudeCodeRunner(exec)
	default:
		return NewGenericCLIRunner(exec, backend)
	}
}

// BuildTaskPrompt generates a short task instruction for a headless agent.
func BuildTaskPrompt(role, agentName, taskID, inboxDir, outboxDir string) string {
	return fmt.Sprintf(
		"당신은 %s입니다.\ninbox 파일을 읽고 태스크를 수행하세요.\ninbox: %s/%s/%s.task.json\n완료 후 결과를 outbox에 JSON으로 작성하세요.\noutbox: %s/%s/%s.result.json",
		role, inboxDir, agentName, taskID, outboxDir, agentName, taskID,
	)
}

// PrepareWorkDir sets up the working directory for an agent.
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
			return wm.Remove(projectDir, wtPath)
		}
		return nil
	}

	return wtPath, cleanup, nil
}
