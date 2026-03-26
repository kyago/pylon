package orchestrator

import (
	"testing"
)

func TestNewPipeline(t *testing.T) {
	p := NewPipeline("test-1", 2)

	if p.ID != "test-1" {
		t.Errorf("expected ID 'test-1', got %q", p.ID)
	}
	if p.CurrentStage != StageInit {
		t.Errorf("expected stage init, got %q", p.CurrentStage)
	}
	if p.MaxAttempts != 2 {
		t.Errorf("expected max_attempts 2, got %d", p.MaxAttempts)
	}
}

func TestNewPipeline_DefaultMaxAttempts(t *testing.T) {
	p := NewPipeline("test-1", 0)
	if p.MaxAttempts != 2 {
		t.Errorf("expected default max_attempts 2, got %d", p.MaxAttempts)
	}
}

func TestPipeline_ValidTransitions(t *testing.T) {
	p := NewPipeline("test-1", 2)

	// Default path: pr_creation is skipped (opt-in only)
	transitions := []Stage{
		StagePOConversation,
		StageArchitectAnalysis,
		StagePMTaskBreakdown,
		StageTaskReview,
		StageAgentExecuting,
		StageVerification,
		StageCompleted,
	}

	for _, stage := range transitions {
		if !p.CanTransition(stage) {
			t.Errorf("expected valid transition %s → %s", p.CurrentStage, stage)
		}
		if err := p.Transition(stage); err != nil {
			t.Errorf("transition %s failed: %v", stage, err)
		}
	}

	if len(p.History) != len(transitions) {
		t.Errorf("expected %d history entries, got %d", len(transitions), len(p.History))
	}
}

func TestPipeline_ValidTransitions_WithPRCreation(t *testing.T) {
	p := NewPipeline("test-pr", 2)

	// Full path with opt-in pr_creation
	transitions := []Stage{
		StagePOConversation,
		StageArchitectAnalysis,
		StagePMTaskBreakdown,
		StageTaskReview,
		StageAgentExecuting,
		StageVerification,
		StagePRCreation,
		StagePOValidation,
		StageWikiUpdate,
		StageCompleted,
	}

	for _, stage := range transitions {
		if !p.CanTransition(stage) {
			t.Errorf("expected valid transition %s → %s", p.CurrentStage, stage)
		}
		if err := p.Transition(stage); err != nil {
			t.Errorf("transition %s failed: %v", stage, err)
		}
	}

	if len(p.History) != len(transitions) {
		t.Errorf("expected %d history entries, got %d", len(transitions), len(p.History))
	}
}

func TestPipeline_VerificationSkipsToPOValidation(t *testing.T) {
	p := NewPipeline("test-skip-pr", 2)
	p.CurrentStage = StageVerification

	// verification → po_validation (skipping pr_creation)
	if !p.CanTransition(StagePOValidation) {
		t.Error("should allow verification → po_validation (skip pr_creation)")
	}
	if err := p.Transition(StagePOValidation); err != nil {
		t.Fatalf("transition to po_validation failed: %v", err)
	}
}

func TestPipeline_VerificationSkipsToWikiUpdate(t *testing.T) {
	p := NewPipeline("test-skip-pr-wiki", 2)
	p.CurrentStage = StageVerification

	// verification → wiki_update (skipping pr_creation)
	if !p.CanTransition(StageWikiUpdate) {
		t.Error("should allow verification → wiki_update (skip pr_creation)")
	}
}

func TestPipeline_InvalidTransition(t *testing.T) {
	p := NewPipeline("test-1", 2)

	// Can't go from init to completed directly
	if p.CanTransition(StageCompleted) {
		t.Error("should not allow init → completed")
	}

	err := p.Transition(StageCompleted)
	if err == nil {
		t.Error("expected error for invalid transition")
	}
}

func TestPipeline_FailFromAnyStage(t *testing.T) {
	stages := []Stage{
		StageInit,
		StagePOConversation,
		StageArchitectAnalysis,
		StagePMTaskBreakdown,
		StageTaskReview,
		StageAgentExecuting,
		StageVerification,
		StagePRCreation,
		StagePOValidation,
		StageWikiUpdate,
	}

	for _, stage := range stages {
		p := &Pipeline{CurrentStage: stage}
		if !p.CanTransition(StageFailed) {
			t.Errorf("should allow %s → failed", stage)
		}
	}
}

func TestPipeline_RetryLimit(t *testing.T) {
	p := NewPipeline("test-1", 2) // MaxAttempts=2 → 2회 재시도 허용

	p.CurrentStage = StageVerification

	// First retry (Attempts: 0 < 2 → 허용, Attempts becomes 1)
	if err := p.Transition(StageAgentExecuting); err != nil {
		t.Fatalf("first retry should succeed: %v", err)
	}
	if p.Attempts != 1 {
		t.Fatalf("expected Attempts=1 after first retry, got %d", p.Attempts)
	}

	p.CurrentStage = StageVerification

	// Second retry (Attempts: 1 < 2 → 허용, Attempts becomes 2)
	if err := p.Transition(StageAgentExecuting); err != nil {
		t.Fatalf("second retry should succeed: %v", err)
	}
	if p.Attempts != 2 {
		t.Fatalf("expected Attempts=2 after second retry, got %d", p.Attempts)
	}

	p.CurrentStage = StageVerification

	// Third retry should fail (Attempts: 2 >= 2 → 차단, Attempts stays 2)
	err := p.Transition(StageAgentExecuting)
	if err == nil {
		t.Error("expected error for max retry attempts exceeded")
	}
	if p.Attempts != 2 {
		t.Fatalf("expected Attempts=2 after rejected retry, got %d", p.Attempts)
	}
}

func TestPipeline_Snapshot_Load(t *testing.T) {
	p := NewPipeline("test-1", 2)
	p.Transition(StagePOConversation)
	p.Agents["backend-dev"] = AgentStatus{
		TaskID:      "task-1",
		AgentID:     "backend-dev",
		Status:      AgentStatusRunning,
	}

	data, err := p.Snapshot()
	if err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}

	loaded, err := LoadPipeline(data)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if loaded.CurrentStage != StagePOConversation {
		t.Errorf("expected stage po_conversation, got %q", loaded.CurrentStage)
	}
	if len(loaded.History) != 1 {
		t.Errorf("expected 1 history entry, got %d", len(loaded.History))
	}
	if loaded.Agents["backend-dev"].AgentID != "backend-dev" {
		t.Error("agent status not preserved")
	}
}

func TestPipeline_PMTaskBreakdown_TransitionsToTaskReview(t *testing.T) {
	p := NewPipeline("test-tr", 2)
	p.Transition(StagePOConversation)
	p.Transition(StageArchitectAnalysis)
	p.Transition(StagePMTaskBreakdown)

	// pm_task_breakdown should NOT transition directly to agent_executing
	if p.CanTransition(StageAgentExecuting) {
		t.Error("pm_task_breakdown should not transition directly to agent_executing")
	}

	// pm_task_breakdown should transition to task_review
	if !p.CanTransition(StageTaskReview) {
		t.Error("pm_task_breakdown should transition to task_review")
	}

	if err := p.Transition(StageTaskReview); err != nil {
		t.Fatalf("transition to task_review failed: %v", err)
	}

	// task_review should transition to agent_executing
	if !p.CanTransition(StageAgentExecuting) {
		t.Error("task_review should transition to agent_executing")
	}
}

func TestPipeline_TaskGraph_Snapshot(t *testing.T) {
	p := NewPipeline("test-tg", 2)
	p.TaskGraph = &TaskGraph{
		Tasks: []TaskItem{
			{ID: "t1", Description: "task 1"},
			{ID: "t2", Description: "task 2", DependsOn: []string{"t1"}},
		},
	}

	data, err := p.Snapshot()
	if err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}

	loaded, err := LoadPipeline(data)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if loaded.TaskGraph == nil {
		t.Fatal("TaskGraph should be preserved after snapshot/load")
	}
	if len(loaded.TaskGraph.Tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(loaded.TaskGraph.Tasks))
	}
	if loaded.TaskGraph.Tasks[1].DependsOn[0] != "t1" {
		t.Errorf("expected dependency on t1, got %v", loaded.TaskGraph.Tasks[1].DependsOn)
	}
}

func TestPipeline_TaskGraph_OmitEmpty(t *testing.T) {
	p := NewPipeline("test-nil-tg", 2)
	// TaskGraph is nil by default

	data, err := p.Snapshot()
	if err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}

	loaded, err := LoadPipeline(data)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if loaded.TaskGraph != nil {
		t.Error("nil TaskGraph should remain nil after snapshot/load")
	}
}

func TestPipeline_IsTerminal(t *testing.T) {
	p := NewPipeline("test-1", 2)
	if p.IsTerminal() {
		t.Error("init should not be terminal")
	}

	p.CurrentStage = StageCompleted
	if !p.IsTerminal() {
		t.Error("completed should be terminal")
	}

	p.CurrentStage = StageFailed
	if !p.IsTerminal() {
		t.Error("failed should be terminal")
	}
}

func TestPipeline_NewFieldsSerialization(t *testing.T) {
	p := NewPipeline("test-new-fields", 2)
	p.WorkflowName = "bugfix"
	p.Status = StatusPaused
	p.PausedAtStage = StageAgentExecuting

	data, err := p.Snapshot()
	if err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}

	loaded, err := LoadPipeline(data)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if loaded.WorkflowName != "bugfix" {
		t.Errorf("expected workflow_name 'bugfix', got %q", loaded.WorkflowName)
	}
	if loaded.Status != StatusPaused {
		t.Errorf("expected status 'paused', got %q", loaded.Status)
	}
	if loaded.PausedAtStage != StageAgentExecuting {
		t.Errorf("expected paused_at_stage 'agent_executing', got %q", loaded.PausedAtStage)
	}
}

func TestNewPipeline_DefaultStatus(t *testing.T) {
	p := NewPipeline("test-default-status", 2)
	if p.Status != StatusRunning {
		t.Errorf("expected default status 'running', got %q", p.Status)
	}
}

func TestLoadPipeline_DefaultStatusWhenEmpty(t *testing.T) {
	// Simulate an older snapshot without the "status" field
	data := []byte(`{"pipeline_id":"old-1","current_stage":"agent_executing","created_at":"2024-01-01T00:00:00Z"}`)
	p, err := LoadPipeline(data)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if p.Status != StatusRunning {
		t.Errorf("expected default status 'running' for legacy snapshot, got %q", p.Status)
	}
}

func TestPipeline_TransitionToCompleted_SetsStatus(t *testing.T) {
	p := NewPipeline("test-status", 2)
	// Walk through default path (no pr_creation)
	for _, s := range []Stage{StagePOConversation, StageArchitectAnalysis, StagePMTaskBreakdown, StageTaskReview, StageAgentExecuting, StageVerification, StageCompleted} {
		if err := p.Transition(s); err != nil {
			t.Fatalf("transition to %s failed: %v", s, err)
		}
	}
	if p.Status != StatusCompleted {
		t.Errorf("expected status 'completed' after reaching StageCompleted, got %q", p.Status)
	}
}

func TestPipeline_TransitionToFailed_SetsStatus(t *testing.T) {
	p := NewPipeline("test-fail-status", 2)
	if err := p.Transition(StageFailed); err != nil {
		t.Fatalf("transition to failed: %v", err)
	}
	if p.Status != StatusFailed {
		t.Errorf("expected status 'failed' after reaching StageFailed, got %q", p.Status)
	}
}

func TestPipeline_EmptyNewFields_OmitEmpty(t *testing.T) {
	p := NewPipeline("test-omit", 2)
	// WorkflowName and PausedAtStage are empty → omitempty

	data, err := p.Snapshot()
	if err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}

	loaded, err := LoadPipeline(data)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if loaded.WorkflowName != "" {
		t.Errorf("expected empty workflow_name, got %q", loaded.WorkflowName)
	}
	if loaded.PausedAtStage != "" {
		t.Errorf("expected empty paused_at_stage, got %q", loaded.PausedAtStage)
	}
}
