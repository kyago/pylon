package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/kyago/pylon/internal/agent"
	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/executor"
)

var _ executor.ProcessExecutor = (*testExecutor)(nil)

// testExecutor is a mock executor for loop tests.
type testExecutor struct {
	runCalls  []executor.ExecConfig
	exitCode  int
	stdout    string
	stderr    string
	err       error
}

func (m *testExecutor) ExecInteractive(cfg executor.ExecConfig) error {
	return fmt.Errorf("interactive not supported in test")
}

func (m *testExecutor) RunHeadless(cfg executor.ExecConfig) (*executor.ExecResult, error) {
	m.runCalls = append(m.runCalls, cfg)
	if m.err != nil {
		return nil, m.err
	}
	return &executor.ExecResult{
		ExitCode: m.exitCode,
		Stdout:   m.stdout,
		Stderr:   m.stderr,
	}, nil
}

func newTestConfig() *config.Config {
	cfg, _ := config.ParseConfig([]byte("version: '1'"))
	return cfg
}

func newTestLoopConfig(workDir string, exec executor.ProcessExecutor) LoopConfig {
	return LoopConfig{
		Config:      newTestConfig(),
		WorkDir:     workDir,
		PipelineID:  "test-pipeline",
		Requirement: "테스트 요구사항",
		Branch:      "task/test-branch",
		Runner:      agent.NewRunner(exec),
		Agents: map[string]*config.AgentConfig{
			"architect": {
				Name: "architect", Role: "아키텍트",
				PermissionMode: "acceptEdits", MaxTurns: 10,
			},
			"pm": {
				Name: "pm", Role: "프로젝트 매니저",
				PermissionMode: "acceptEdits", MaxTurns: 10,
			},
			"backend-dev": {
				Name: "backend-dev", Role: "백엔드 개발자",
				PermissionMode: "acceptEdits", MaxTurns: 30,
			},
		},
		Projects: []config.ProjectInfo{
			{Name: "test-project", Path: workDir},
		},
	}
}

func TestNewLoop(t *testing.T) {
	dir := t.TempDir()
	exec := &testExecutor{}

	loop := NewLoop(newTestLoopConfig(dir, exec))
	if loop == nil {
		t.Fatal("NewLoop returned nil")
	}
	if loop.orch == nil {
		t.Error("orchestrator should be set")
	}
	if loop.watcher == nil {
		t.Error("watcher should be set")
	}
}

func TestLoop_Run_POConversation_ReturnsInteractive(t *testing.T) {
	dir := t.TempDir()
	exec := &testExecutor{}
	loop := NewLoop(newTestLoopConfig(dir, exec))

	err := loop.Run(context.Background())
	if !errors.Is(err, ErrInteractiveRequired) {
		t.Errorf("expected ErrInteractiveRequired, got: %v", err)
	}
}

func TestLoop_Run_HeadlessFromArchitect(t *testing.T) {
	dir := t.TempDir()
	exec := &testExecutor{exitCode: 0}
	lcfg := newTestLoopConfig(dir, exec)
	loop := NewLoop(lcfg)

	// Manually set pipeline to architect stage
	loop.orch.StartPipeline("test-pipeline")
	loop.orch.TransitionTo(StagePOConversation)
	loop.orch.TransitionTo(StageArchitectAnalysis)

	// Run from architect stage — should execute architect, then pm, then dev agents
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := loop.Run(ctx)

	// It will try to run agents and likely fail at PR creation (no git repo)
	// but it should at least execute the architect and PM agents
	if err == nil {
		t.Fatal("expected error (no git repo for PR)")
	}

	// Verify agents were called
	if len(exec.runCalls) < 2 {
		t.Errorf("expected at least 2 agent calls (architect + pm), got %d", len(exec.runCalls))
	}
}

func TestLoop_Run_ContextCancellation(t *testing.T) {
	dir := t.TempDir()
	exec := &testExecutor{exitCode: 0}
	loop := NewLoop(newTestLoopConfig(dir, exec))

	// Start pipeline at a headless stage
	loop.orch.StartPipeline("test-pipeline")
	loop.orch.TransitionTo(StagePOConversation)
	loop.orch.TransitionTo(StageArchitectAnalysis)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := loop.Run(ctx)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestLoop_Run_AgentExecutionFails(t *testing.T) {
	dir := t.TempDir()
	exec := &testExecutor{err: fmt.Errorf("agent crashed")}
	loop := NewLoop(newTestLoopConfig(dir, exec))

	loop.orch.StartPipeline("test-pipeline")
	loop.orch.TransitionTo(StagePOConversation)
	loop.orch.TransitionTo(StageArchitectAnalysis)

	err := loop.Run(context.Background())
	if err == nil {
		t.Fatal("expected error for failed agent")
	}
}

func TestLoop_Run_AgentNonZeroExit(t *testing.T) {
	dir := t.TempDir()
	exec := &testExecutor{exitCode: 1, stderr: "error output"}
	loop := NewLoop(newTestLoopConfig(dir, exec))

	loop.orch.StartPipeline("test-pipeline")
	loop.orch.TransitionTo(StagePOConversation)
	loop.orch.TransitionTo(StageArchitectAnalysis)

	err := loop.Run(context.Background())
	if err == nil {
		t.Fatal("expected error for non-zero exit code")
	}
}

func TestLoop_findAgent(t *testing.T) {
	dir := t.TempDir()
	exec := &testExecutor{}
	loop := NewLoop(newTestLoopConfig(dir, exec))

	a := loop.findAgent("architect")
	if a == nil {
		t.Fatal("expected to find architect")
	}
	if a.Name != "architect" {
		t.Errorf("name = %q, want architect", a.Name)
	}

	missing := loop.findAgent("nonexistent")
	if missing != nil {
		t.Error("expected nil for missing agent")
	}
}

func TestLoop_findDevAgents(t *testing.T) {
	dir := t.TempDir()
	exec := &testExecutor{}
	loop := NewLoop(newTestLoopConfig(dir, exec))

	devs := loop.findDevAgents()
	if len(devs) != 1 {
		t.Errorf("expected 1 dev agent, got %d", len(devs))
	}
}

func TestLoop_buildPRBody(t *testing.T) {
	dir := t.TempDir()
	exec := &testExecutor{}
	loop := NewLoop(newTestLoopConfig(dir, exec))
	loop.orch.StartPipeline("test-pipeline")

	body := loop.buildPRBody()
	if body == "" {
		t.Fatal("PR body should not be empty")
	}
}

func TestLoop_Verification_NoVerifyYml(t *testing.T) {
	dir := t.TempDir()
	exec := &testExecutor{exitCode: 0}
	loop := NewLoop(newTestLoopConfig(dir, exec))

	loop.orch.StartPipeline("test-pipeline")
	loop.orch.TransitionTo(StagePOConversation)
	loop.orch.TransitionTo(StageArchitectAnalysis)
	loop.orch.TransitionTo(StagePMTaskBreakdown)
	loop.orch.TransitionTo(StageAgentExecuting)
	loop.orch.TransitionTo(StageVerification)

	// Run verification — should skip since no verify.yml exists
	err := loop.runVerification(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have transitioned to PR creation
	if loop.orch.Pipeline.CurrentStage != StagePRCreation {
		t.Errorf("stage = %s, want pr_creation", loop.orch.Pipeline.CurrentStage)
	}
}
