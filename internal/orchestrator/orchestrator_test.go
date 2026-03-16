package orchestrator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/store"
)

func setupTestOrchestrator(t *testing.T) (*Orchestrator, string) {
	t.Helper()
	dir := t.TempDir()

	s, err := store.NewStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	if err := s.Migrate(); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	cfg := &config.Config{
		Runtime: config.RuntimeConfig{
			MaxAttempts: 3,
		},
	}

	orch := NewOrchestrator(cfg, s, dir)
	return orch, dir
}

func TestNewOrchestrator(t *testing.T) {
	orch, dir := setupTestOrchestrator(t)

	if orch.Config == nil {
		t.Fatal("config should not be nil")
	}
	if orch.WorkDir != dir {
		t.Errorf("workdir = %s, want %s", orch.WorkDir, dir)
	}
	if orch.Pipeline != nil {
		t.Error("pipeline should be nil initially")
	}
}

func TestStartPipeline(t *testing.T) {
	orch, _ := setupTestOrchestrator(t)

	if err := orch.StartPipeline("test-001"); err != nil {
		t.Fatalf("StartPipeline failed: %v", err)
	}

	if orch.Pipeline == nil {
		t.Fatal("pipeline should not be nil after start")
	}
	if orch.Pipeline.ID != "test-001" {
		t.Errorf("pipeline ID = %s, want test-001", orch.Pipeline.ID)
	}
	if orch.Pipeline.CurrentStage != StageInit {
		t.Errorf("stage = %s, want %s", orch.Pipeline.CurrentStage, StageInit)
	}

	// Verify state was persisted to SQLite
	rec, err := orch.Store.GetPipeline("test-001")
	if err != nil {
		t.Fatalf("failed to get pipeline from store: %v", err)
	}
	if rec == nil {
		t.Fatal("pipeline should be persisted in SQLite after StartPipeline")
	}
	if rec.Stage != string(StageInit) {
		t.Errorf("stored stage = %s, want %s", rec.Stage, StageInit)
	}
}

func TestTransitionTo(t *testing.T) {
	orch, _ := setupTestOrchestrator(t)

	if err := orch.StartPipeline("test-002"); err != nil {
		t.Fatalf("StartPipeline failed: %v", err)
	}

	if err := orch.TransitionTo(StagePOConversation); err != nil {
		t.Fatalf("TransitionTo failed: %v", err)
	}
	if orch.Pipeline.CurrentStage != StagePOConversation {
		t.Errorf("stage = %s, want %s", orch.Pipeline.CurrentStage, StagePOConversation)
	}
}

func TestTransitionTo_NoPipeline(t *testing.T) {
	orch, _ := setupTestOrchestrator(t)

	if err := orch.TransitionTo(StagePOConversation); err == nil {
		t.Error("expected error when no pipeline active")
	}
}

func TestRecover_NoState(t *testing.T) {
	orch, _ := setupTestOrchestrator(t)

	if _, err := orch.Recover(); err != nil {
		t.Fatalf("Recover with no state should succeed: %v", err)
	}
	if orch.Pipeline != nil {
		t.Error("pipeline should remain nil when no state to recover")
	}
}

func TestRecover_WithState(t *testing.T) {
	orch, dir := setupTestOrchestrator(t)

	// Create a pipeline and save state
	if err := orch.StartPipeline("recover-001"); err != nil {
		t.Fatalf("StartPipeline failed: %v", err)
	}
	if err := orch.TransitionTo(StagePOConversation); err != nil {
		t.Fatalf("TransitionTo failed: %v", err)
	}

	// Add a mock agent status
	orch.Pipeline.Agents["test-agent"] = AgentStatus{
		AgentID: "test-agent",
		Status:  "running",
	}
	if err := orch.savePipelineState(); err != nil {
		t.Fatalf("savePipelineState failed: %v", err)
	}

	// Create a new orchestrator and recover
	orch2 := NewOrchestrator(orch.Config, orch.Store, dir)
	if _, err := orch2.Recover(); err != nil {
		t.Fatalf("Recover failed: %v", err)
	}

	if orch2.Pipeline == nil {
		t.Fatal("pipeline should be recovered")
	}
	if orch2.Pipeline.ID != "recover-001" {
		t.Errorf("pipeline ID = %s, want recover-001", orch2.Pipeline.ID)
	}
	if orch2.Pipeline.CurrentStage != StagePOConversation {
		t.Errorf("stage = %s, want %s", orch2.Pipeline.CurrentStage, StagePOConversation)
	}
}

func TestRecover_TerminalState(t *testing.T) {
	orch, _ := setupTestOrchestrator(t)

	// Create completed pipeline in SQLite
	pipeline := NewPipeline("done-001", 3)
	pipeline.CurrentStage = StageCompleted
	data, _ := pipeline.Snapshot()

	orch.Store.UpsertPipeline(&store.PipelineRecord{
		PipelineID: "done-001",
		Stage:      string(StageCompleted),
		StateJSON:  string(data),
		UpdatedAt:  time.Now(),
	})

	orch.SetPipelineID("done-001")
	if _, err := orch.Recover(); err != nil {
		t.Fatalf("Recover failed: %v", err)
	}
	if orch.Pipeline != nil {
		t.Error("pipeline should remain nil for terminal state")
	}
}

func TestRecover_ReturnsUnprocessedResults(t *testing.T) {
	orch, dir := setupTestOrchestrator(t)

	// Create an active pipeline
	if err := orch.StartPipeline("unprocessed-001"); err != nil {
		t.Fatalf("StartPipeline failed: %v", err)
	}
	if err := orch.TransitionTo(StagePOConversation); err != nil {
		t.Fatalf("TransitionTo failed: %v", err)
	}

	// Create an unprocessed outbox result file
	outboxDir := filepath.Join(dir, ".pylon", "runtime", "outbox", "backend-dev")
	if err := os.MkdirAll(outboxDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	resultFile := filepath.Join(outboxDir, "task-123.result.json")
	resultJSON := `{"type":"result","from":"backend-dev","to":"orchestrator","body":{"summary":"done"}}`
	if err := os.WriteFile(resultFile, []byte(resultJSON), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Recover with a new orchestrator
	orch2 := NewOrchestrator(orch.Config, orch.Store, dir)
	orch2.SetPipelineID("unprocessed-001")

	unprocessed, err := orch2.Recover()
	if err != nil {
		t.Fatalf("Recover failed: %v", err)
	}

	if len(unprocessed) != 1 {
		t.Fatalf("expected 1 unprocessed result, got %d", len(unprocessed))
	}
	if unprocessed[0].AgentName != "backend-dev" {
		t.Errorf("agent name = %s, want backend-dev", unprocessed[0].AgentName)
	}
	if unprocessed[0].TaskID != "task-123" {
		t.Errorf("task ID = %s, want task-123", unprocessed[0].TaskID)
	}
}

func TestGetStatus(t *testing.T) {
	orch, _ := setupTestOrchestrator(t)

	// No pipeline
	status := orch.GetStatus()
	if status["pipeline"] != "none" {
		t.Errorf("expected pipeline=none, got %v", status["pipeline"])
	}

	// With pipeline
	orch.StartPipeline("status-001")
	status = orch.GetStatus()
	if status["pipeline_id"] != "status-001" {
		t.Errorf("pipeline_id = %v, want status-001", status["pipeline_id"])
	}
	if status["stage"] != StageInit {
		t.Errorf("stage = %v, want %s", status["stage"], StageInit)
	}
}

func TestGetStatusJSON(t *testing.T) {
	orch, _ := setupTestOrchestrator(t)
	orch.StartPipeline("json-001")

	jsonStr, err := orch.GetStatusJSON()
	if err != nil {
		t.Fatalf("GetStatusJSON failed: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if parsed["pipeline_id"] != "json-001" {
		t.Errorf("pipeline_id = %v, want json-001", parsed["pipeline_id"])
	}
}
