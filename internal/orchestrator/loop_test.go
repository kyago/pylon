package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/kyago/pylon/internal/agent"
	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/executor"
	"github.com/kyago/pylon/internal/store"
)

var _ executor.ProcessExecutor = (*testExecutor)(nil)

// testExecutor is a mock executor for loop tests.
type testExecutor struct {
	mu        sync.Mutex
	runCalls  []executor.ExecConfig
	exitCode  int
	stdout    string
	stderr    string
	err       error
}

func (m *testExecutor) ExecInteractive(cfg executor.ExecConfig) error {
	return fmt.Errorf("interactive not supported in test")
}

func (m *testExecutor) RunInteractive(cfg executor.ExecConfig) error {
	return fmt.Errorf("run interactive not supported in test")
}

func (m *testExecutor) RunHeadless(cfg executor.ExecConfig) (*executor.ExecResult, error) {
	m.mu.Lock()
	m.runCalls = append(m.runCalls, cfg)
	m.mu.Unlock()
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

func newTestStore(t testing.TB) *store.Store {
	t.Helper()
	s, err := store.NewStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}
	if err := s.Migrate(); err != nil {
		t.Fatalf("failed to migrate test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func newTestLoopConfig(workDir string, exec executor.ProcessExecutor, extras ...testing.TB) LoopConfig {
	lcfg := LoopConfig{
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
	if len(extras) > 0 {
		lcfg.Store = newTestStore(extras[0])
	}
	return lcfg
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
	lcfg.Config.Runtime.AutoApproveTaskReview = true
	loop := NewLoop(lcfg)

	// Manually set pipeline to architect stage
	loop.orch.StartPipeline("test-pipeline")
	loop.orch.TransitionTo(StagePOConversation)
	loop.orch.TransitionTo(StageArchitectAnalysis)

	// Run from architect stage — should execute architect, then pm, task_review (auto-approve), then dev agents.
	// PR creation failure is non-fatal; pipeline skips to wiki update and completes.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := loop.Run(ctx)

	if err != nil {
		t.Fatalf("expected pipeline to complete (PR failure is non-fatal), got: %v", err)
	}

	// Pipeline should reach completed stage (PR skip → wiki update → completed)
	if loop.orch.Pipeline.CurrentStage != StageCompleted {
		t.Errorf("expected completed stage, got %s", loop.orch.Pipeline.CurrentStage)
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

func TestLoop_findDevAgents_ByType(t *testing.T) {
	dir := t.TempDir()
	exec := &testExecutor{}
	lcfg := newTestLoopConfig(dir, exec)

	// Add a custom-named agent with explicit type: dev
	lcfg.Agents["my-custom-dev"] = &config.AgentConfig{
		Name: "my-custom-dev", Role: "커스텀 개발자",
		Type: "dev",
	}

	loop := NewLoop(lcfg)
	devs := loop.findDevAgents()

	// Should find both backend-dev (inferred) and my-custom-dev (explicit)
	if len(devs) != 2 {
		t.Errorf("expected 2 dev agents, got %d: %v", len(devs), devs)
	}

	found := make(map[string]bool)
	for _, d := range devs {
		found[d] = true
	}
	if !found["backend-dev"] {
		t.Error("expected backend-dev in dev agents")
	}
	if !found["my-custom-dev"] {
		t.Error("expected my-custom-dev in dev agents")
	}
}

func TestLoop_findDevAgents_ExplicitTypeOverride(t *testing.T) {
	dir := t.TempDir()
	exec := &testExecutor{}
	lcfg := newTestLoopConfig(dir, exec)

	// Override backend-dev with type != dev → should NOT be found
	lcfg.Agents["backend-dev"].Type = "infra"

	loop := NewLoop(lcfg)
	devs := loop.findDevAgents()

	if len(devs) != 0 {
		t.Errorf("expected 0 dev agents when type overridden, got %d: %v", len(devs), devs)
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

func TestLoop_Run_ParallelDevAgents(t *testing.T) {
	dir := t.TempDir()
	exec := &testExecutor{exitCode: 0}
	lcfg := newTestLoopConfig(dir, exec)
	// Add a second dev agent
	lcfg.Agents["frontend-dev"] = &config.AgentConfig{
		Name: "frontend-dev", Role: "프론트엔드 개발자",
		PermissionMode: "acceptEdits", MaxTurns: 30,
	}
	loop := NewLoop(lcfg)

	loop.orch.StartPipeline("test-pipeline")
	loop.orch.TransitionTo(StagePOConversation)
	loop.orch.TransitionTo(StageArchitectAnalysis)
	loop.orch.TransitionTo(StagePMTaskBreakdown)
	loop.orch.TransitionTo(StageTaskReview)
	loop.orch.TransitionTo(StageAgentExecuting)

	err := loop.runAgentExecution(context.Background())

	// Should execute both dev agents (backend-dev + frontend-dev)
	if err != nil {
		// PR creation will fail (no git), but agents should have been called
		t.Logf("error (expected in test): %v", err)
	}
	if len(exec.runCalls) < 2 {
		t.Errorf("expected at least 2 agent calls, got %d", len(exec.runCalls))
	}
}

func TestLoop_runTaskReview_AutoApprove(t *testing.T) {
	dir := t.TempDir()
	exec := &testExecutor{exitCode: 0}
	lcfg := newTestLoopConfig(dir, exec)
	lcfg.Config.Runtime.AutoApproveTaskReview = true
	loop := NewLoop(lcfg)

	loop.orch.StartPipeline("review-auto")
	loop.orch.TransitionTo(StagePOConversation)
	loop.orch.TransitionTo(StageArchitectAnalysis)
	loop.orch.TransitionTo(StagePMTaskBreakdown)
	loop.orch.TransitionTo(StageTaskReview)

	err := loop.runTaskReview(context.Background())
	if err != nil {
		t.Fatalf("expected auto-approve, got: %v", err)
	}
	if loop.orch.Pipeline.CurrentStage != StageAgentExecuting {
		t.Errorf("stage = %s, want agent_executing", loop.orch.Pipeline.CurrentStage)
	}
}

func TestLoop_runTaskReview_NoPOAgent(t *testing.T) {
	dir := t.TempDir()
	exec := &testExecutor{exitCode: 0}
	lcfg := newTestLoopConfig(dir, exec)
	// AutoApproveTaskReview=false (default), no PO agent configured
	loop := NewLoop(lcfg)

	loop.orch.StartPipeline("review-no-po")
	loop.orch.TransitionTo(StagePOConversation)
	loop.orch.TransitionTo(StageArchitectAnalysis)
	loop.orch.TransitionTo(StagePMTaskBreakdown)
	loop.orch.TransitionTo(StageTaskReview)

	err := loop.runTaskReview(context.Background())
	if err != nil {
		t.Fatalf("expected auto-approve when no PO agent, got: %v", err)
	}
	if loop.orch.Pipeline.CurrentStage != StageAgentExecuting {
		t.Errorf("stage = %s, want agent_executing", loop.orch.Pipeline.CurrentStage)
	}
}

func TestLoop_runTaskReview_InteractiveRequired(t *testing.T) {
	dir := t.TempDir()
	exec := &testExecutor{exitCode: 0}
	lcfg := newTestLoopConfig(dir, exec)
	// Add PO agent and don't auto-approve
	lcfg.Agents["po"] = &config.AgentConfig{
		Name: "po", Role: "프로덕트 오너",
	}
	loop := NewLoop(lcfg)

	loop.orch.StartPipeline("review-interactive")
	loop.orch.TransitionTo(StagePOConversation)
	loop.orch.TransitionTo(StageArchitectAnalysis)
	loop.orch.TransitionTo(StagePMTaskBreakdown)
	loop.orch.TransitionTo(StageTaskReview)

	err := loop.runTaskReview(context.Background())
	if !errors.Is(err, ErrInteractiveRequired) {
		t.Errorf("expected ErrInteractiveRequired, got: %v", err)
	}
}

func TestLoop_Run_HeadlessFromArchitect_WithTaskReview(t *testing.T) {
	dir := t.TempDir()
	exec := &testExecutor{exitCode: 0}
	lcfg := newTestLoopConfig(dir, exec)
	lcfg.Config.Runtime.AutoApproveTaskReview = true
	loop := NewLoop(lcfg)

	loop.orch.StartPipeline("tr-headless")
	loop.orch.TransitionTo(StagePOConversation)
	loop.orch.TransitionTo(StageArchitectAnalysis)

	err := loop.Run(context.Background())
	if err != nil {
		t.Fatalf("expected pipeline to complete, got: %v", err)
	}

	if loop.orch.Pipeline.CurrentStage != StageCompleted {
		t.Errorf("expected completed stage, got %s", loop.orch.Pipeline.CurrentStage)
	}
}

func TestLoop_runAgentExecution_WaveBased(t *testing.T) {
	dir := t.TempDir()
	exec := &testExecutor{exitCode: 0}
	lcfg := newTestLoopConfig(dir, exec)
	loop := NewLoop(lcfg)

	loop.orch.StartPipeline("wave-test")
	loop.orch.TransitionTo(StagePOConversation)
	loop.orch.TransitionTo(StageArchitectAnalysis)
	loop.orch.TransitionTo(StagePMTaskBreakdown)
	loop.orch.TransitionTo(StageTaskReview)
	loop.orch.TransitionTo(StageAgentExecuting)

	// Set up a task graph with 2 waves
	loop.orch.Pipeline.TaskGraph = &TaskGraph{
		Tasks: []TaskItem{
			{ID: "t1", Description: "first", AgentName: "backend-dev"},
			{ID: "t2", Description: "second", AgentName: "backend-dev", DependsOn: []string{"t1"}},
		},
	}

	err := loop.runAgentExecution(context.Background())
	if err != nil {
		t.Logf("error (may be expected in test): %v", err)
	}

	// Should have executed 2 agent calls (one per wave)
	if len(exec.runCalls) < 2 {
		t.Errorf("expected at least 2 agent calls for 2 waves, got %d", len(exec.runCalls))
	}
}

func TestLoop_runAgentExecution_LegacyFallback(t *testing.T) {
	dir := t.TempDir()
	exec := &testExecutor{exitCode: 0}
	lcfg := newTestLoopConfig(dir, exec)
	loop := NewLoop(lcfg)

	loop.orch.StartPipeline("legacy-test")
	loop.orch.TransitionTo(StagePOConversation)
	loop.orch.TransitionTo(StageArchitectAnalysis)
	loop.orch.TransitionTo(StagePMTaskBreakdown)
	loop.orch.TransitionTo(StageTaskReview)
	loop.orch.TransitionTo(StageAgentExecuting)

	// No TaskGraph → legacy parallel path
	err := loop.runAgentExecution(context.Background())
	if err != nil {
		t.Logf("error (may be expected in test): %v", err)
	}

	// Should have at least 1 agent call
	if len(exec.runCalls) < 1 {
		t.Errorf("expected at least 1 agent call, got %d", len(exec.runCalls))
	}
}

func TestLoop_extractTaskGraph_Nil(t *testing.T) {
	dir := t.TempDir()
	exec := &testExecutor{}
	loop := NewLoop(newTestLoopConfig(dir, exec))

	// No lastResult → nil graph
	graph := loop.extractTaskGraph()
	if graph != nil {
		t.Error("expected nil task graph when no last result")
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
	loop.orch.TransitionTo(StageTaskReview)
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
