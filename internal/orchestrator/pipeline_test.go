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

	transitions := []Stage{
		StagePOConversation,
		StageArchitectAnalysis,
		StagePMTaskBreakdown,
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
