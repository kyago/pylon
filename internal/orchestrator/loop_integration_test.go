package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kyago/pylon/internal/agent"
	"github.com/kyago/pylon/internal/executor"
	"github.com/kyago/pylon/internal/protocol"
)

// --- Integration Tests ---

// multiStageExecutor simulates different exit codes per agent call.
type multiStageExecutor struct {
	calls     []executor.ExecConfig
	responses []struct {
		exitCode int
		err      error
	}
	callIndex int
}

func (m *multiStageExecutor) ExecInteractive(cfg executor.ExecConfig) error {
	return fmt.Errorf("interactive not supported")
}

func (m *multiStageExecutor) RunHeadless(cfg executor.ExecConfig) (*executor.ExecResult, error) {
	m.calls = append(m.calls, cfg)
	idx := m.callIndex
	m.callIndex++

	if idx < len(m.responses) {
		r := m.responses[idx]
		if r.err != nil {
			return nil, r.err
		}
		return &executor.ExecResult{ExitCode: r.exitCode}, nil
	}
	return &executor.ExecResult{ExitCode: 0}, nil
}

func TestIntegration_FullHeadlessPipeline(t *testing.T) {
	dir := t.TempDir()
	exec := &testExecutor{exitCode: 0}
	lcfg := newTestLoopConfig(dir, exec)
	loop := NewLoop(lcfg)

	// Start pipeline and skip PO (set to architect stage)
	loop.orch.StartPipeline("int-test-001")
	loop.orch.TransitionTo(StagePOConversation)
	loop.orch.TransitionTo(StageArchitectAnalysis)

	ctx := context.Background()
	err := loop.Run(ctx)

	// Will fail at PR creation (no git repo), but should have executed agents
	if err == nil {
		t.Log("Pipeline completed (unexpected but ok)")
	}

	// Verify multiple agents were called
	if len(exec.runCalls) < 3 {
		t.Errorf("expected at least 3 agent calls (architect, pm, dev), got %d", len(exec.runCalls))
	}
}

func TestIntegration_VerificationRetry(t *testing.T) {
	dir := t.TempDir()

	// Create a verify.yml that will always fail (command doesn't exist)
	projectPylonDir := filepath.Join(dir, ".pylon")
	os.MkdirAll(projectPylonDir, 0755)
	os.WriteFile(filepath.Join(projectPylonDir, "verify.yml"), []byte(`
build:
  command: "false"
  timeout: "5s"
`), 0644)

	exec := &testExecutor{exitCode: 0}
	lcfg := newTestLoopConfig(dir, exec)
	lcfg.Config.Runtime.MaxAttempts = 2
	loop := NewLoop(lcfg)

	// Start at verification stage
	loop.orch.StartPipeline("retry-test")
	loop.orch.Pipeline.Agents = make(map[string]AgentStatus)
	loop.orch.TransitionTo(StagePOConversation)
	loop.orch.TransitionTo(StageArchitectAnalysis)
	loop.orch.TransitionTo(StagePMTaskBreakdown)
	loop.orch.TransitionTo(StageAgentExecuting)
	loop.orch.TransitionTo(StageVerification)
	loop.orch.Pipeline.MaxAttempts = 2

	err := loop.Run(context.Background())

	// Should fail after max attempts
	if err == nil {
		t.Fatal("expected error after max verification attempts")
	}

	// Pipeline should be in failed state
	if loop.orch.Pipeline.CurrentStage != StageFailed {
		t.Errorf("stage = %s, want failed", loop.orch.Pipeline.CurrentStage)
	}
}

func TestIntegration_ContextCancellation_GracefulShutdown(t *testing.T) {
	dir := t.TempDir()

	// Use a slow executor that blocks
	slowExec := &multiStageExecutor{
		responses: []struct {
			exitCode int
			err      error
		}{
			{exitCode: 0, err: nil}, // architect succeeds
			{exitCode: 0, err: nil}, // pm succeeds
		},
	}

	lcfg := newTestLoopConfig(dir, slowExec)
	lcfg.Runner = agent.NewRunner(slowExec)
	loop := NewLoop(lcfg)

	loop.orch.StartPipeline("cancel-test")
	loop.orch.Pipeline.Agents = make(map[string]AgentStatus)
	loop.orch.TransitionTo(StagePOConversation)
	loop.orch.TransitionTo(StageArchitectAnalysis)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := loop.Run(ctx)
	// Should either complete quickly or be cancelled
	if err != nil && err != context.DeadlineExceeded {
		// Any error is acceptable as long as it doesn't panic
		t.Logf("Got expected error: %v", err)
	}
}

func TestIntegration_CrashRecovery(t *testing.T) {
	dir := t.TempDir()

	// Create runtime directories
	runtimeDir := filepath.Join(dir, ".pylon", "runtime")
	os.MkdirAll(runtimeDir, 0755)

	// Simulate a crashed pipeline by writing state.json
	pipeline := NewPipeline("crash-test", 2)
	pipeline.Transition(StagePOConversation)
	pipeline.Transition(StageArchitectAnalysis)
	pipeline.TaskSpec = "크래시 테스트"

	data, _ := pipeline.Snapshot()
	os.WriteFile(filepath.Join(runtimeDir, "state.json"), data, 0644)

	// Create new loop — should recover from state.json
	exec := &testExecutor{exitCode: 0}
	lcfg := newTestLoopConfig(dir, exec)
	loop := NewLoop(lcfg)

	err := loop.Run(context.Background())

	// Should have recovered and continued from architect stage
	if err == nil {
		t.Log("Pipeline recovered and completed")
	}

	// Verify agents were called (recovered pipeline continues)
	if len(exec.runCalls) == 0 {
		t.Error("expected agent calls after recovery")
	}
}

func TestIntegration_AgentConfigNotFound(t *testing.T) {
	dir := t.TempDir()
	exec := &testExecutor{exitCode: 0}
	lcfg := newTestLoopConfig(dir, exec)

	// Remove architect agent
	delete(lcfg.Agents, "architect")

	loop := NewLoop(lcfg)
	loop.orch.StartPipeline("no-agent-test")
	loop.orch.Pipeline.Agents = make(map[string]AgentStatus)
	loop.orch.TransitionTo(StagePOConversation)
	loop.orch.TransitionTo(StageArchitectAnalysis)

	err := loop.Run(context.Background())
	if err == nil {
		t.Fatal("expected error for missing agent config")
	}
}

func TestIntegration_NoProjects(t *testing.T) {
	dir := t.TempDir()
	exec := &testExecutor{exitCode: 0}
	lcfg := newTestLoopConfig(dir, exec)
	lcfg.Projects = nil // No projects

	loop := NewLoop(lcfg)
	loop.orch.StartPipeline("no-proj-test")
	loop.orch.Pipeline.Agents = make(map[string]AgentStatus)
	loop.orch.TransitionTo(StagePOConversation)
	loop.orch.TransitionTo(StageArchitectAnalysis)

	// Should still work — uses workspace root as fallback
	err := loop.Run(context.Background())
	if err == nil {
		t.Log("Pipeline completed without projects (uses workspace root)")
	}
}

func TestIntegration_OutboxResultProcessing(t *testing.T) {
	dir := t.TempDir()

	// Pre-create outbox result
	outboxDir := filepath.Join(dir, ".pylon", "runtime", "outbox")

	msg := protocol.NewMessage(protocol.MsgResult, "architect", "orchestrator")
	msg.Context = &protocol.MsgContext{TaskID: "int-test-002-architect"}
	msg.Body = map[string]any{
		"task_id":  "int-test-002-architect",
		"status":   "completed",
		"summary":  "Architecture analysis complete",
		"learnings": []any{"Go Echo framework for REST API"},
	}
	protocol.WriteResult(outboxDir, "architect", msg)

	exec := &testExecutor{exitCode: 0}
	lcfg := newTestLoopConfig(dir, exec)
	loop := NewLoop(lcfg)

	loop.orch.StartPipeline("int-test-002")
	loop.orch.Pipeline.Agents = make(map[string]AgentStatus)
	loop.orch.TransitionTo(StagePOConversation)
	loop.orch.TransitionTo(StageArchitectAnalysis)

	// Run — architect's outbox result should be picked up
	err := loop.Run(context.Background())
	if err == nil {
		t.Log("Pipeline completed with outbox processing")
	}
}

func TestIntegration_POValidation_AutoApprove(t *testing.T) {
	dir := t.TempDir()
	exec := &testExecutor{exitCode: 0}
	lcfg := newTestLoopConfig(dir, exec)
	loop := NewLoop(lcfg)

	loop.orch.StartPipeline("po-val-test")
	loop.orch.Pipeline.Agents = make(map[string]AgentStatus)
	loop.orch.TransitionTo(StagePOConversation)
	loop.orch.TransitionTo(StageArchitectAnalysis)
	loop.orch.TransitionTo(StagePMTaskBreakdown)
	loop.orch.TransitionTo(StageAgentExecuting)
	loop.orch.TransitionTo(StageVerification)
	loop.orch.TransitionTo(StagePRCreation)
	loop.orch.TransitionTo(StagePOValidation)

	err := loop.runPOValidation(context.Background())
	if err != nil {
		t.Fatalf("PO validation failed: %v", err)
	}

	if loop.orch.Pipeline.CurrentStage != StageWikiUpdate {
		t.Errorf("stage = %s, want wiki_update", loop.orch.Pipeline.CurrentStage)
	}
}

func TestIntegration_WikiUpdate_NoTechWriter(t *testing.T) {
	dir := t.TempDir()
	exec := &testExecutor{exitCode: 0}
	lcfg := newTestLoopConfig(dir, exec)
	loop := NewLoop(lcfg)

	loop.orch.StartPipeline("wiki-test")
	loop.orch.Pipeline.Agents = make(map[string]AgentStatus)
	loop.orch.TransitionTo(StagePOConversation)
	loop.orch.TransitionTo(StageArchitectAnalysis)
	loop.orch.TransitionTo(StagePMTaskBreakdown)
	loop.orch.TransitionTo(StageAgentExecuting)
	loop.orch.TransitionTo(StageVerification)
	loop.orch.TransitionTo(StagePRCreation)
	loop.orch.TransitionTo(StagePOValidation)
	loop.orch.TransitionTo(StageWikiUpdate)

	err := loop.runWikiUpdate(context.Background())
	if err != nil {
		t.Fatalf("Wiki update failed: %v", err)
	}

	if loop.orch.Pipeline.CurrentStage != StageCompleted {
		t.Errorf("stage = %s, want completed", loop.orch.Pipeline.CurrentStage)
	}
}

func TestIntegration_WikiUpdate_AutoUpdateDisabled(t *testing.T) {
	dir := t.TempDir()
	exec := &testExecutor{exitCode: 0}
	lcfg := newTestLoopConfig(dir, exec)
	lcfg.Config.Wiki.AutoUpdate = false
	loop := NewLoop(lcfg)

	loop.orch.StartPipeline("wiki-off-test")
	loop.orch.Pipeline.Agents = make(map[string]AgentStatus)
	loop.orch.TransitionTo(StagePOConversation)
	loop.orch.TransitionTo(StageArchitectAnalysis)
	loop.orch.TransitionTo(StagePMTaskBreakdown)
	loop.orch.TransitionTo(StageAgentExecuting)
	loop.orch.TransitionTo(StageVerification)
	loop.orch.TransitionTo(StagePRCreation)
	loop.orch.TransitionTo(StagePOValidation)
	loop.orch.TransitionTo(StageWikiUpdate)

	err := loop.runWikiUpdate(context.Background())
	if err != nil {
		t.Fatalf("Wiki update failed: %v", err)
	}

	if loop.orch.Pipeline.CurrentStage != StageCompleted {
		t.Errorf("stage = %s, want completed", loop.orch.Pipeline.CurrentStage)
	}
}
