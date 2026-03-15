package orchestrator

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/store"
)

// setupStateTestOrchestrator creates an orchestrator with an in-memory SQLite store.
func setupStateTestOrchestrator(t *testing.T) (*Orchestrator, string) {
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

func TestSavePipelineState_SQLiteOnly(t *testing.T) {
	orch, dir := setupStateTestOrchestrator(t)

	if err := orch.StartPipeline("sqlite-only-001"); err != nil {
		t.Fatalf("StartPipeline failed: %v", err)
	}

	// Verify SQLite has the pipeline
	rec, err := orch.Store.GetPipeline("sqlite-only-001")
	if err != nil {
		t.Fatalf("GetPipeline failed: %v", err)
	}
	if rec == nil {
		t.Fatal("pipeline should exist in SQLite")
	}
	if rec.Stage != string(StageInit) {
		t.Errorf("stage = %s, want %s", rec.Stage, StageInit)
	}

	// Verify state.json was NOT created
	statePath := filepath.Join(dir, ".pylon", "runtime", "state.json")
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Error("state.json should NOT exist — state is stored in SQLite only")
	}
}

func TestRecover_FromSQLite(t *testing.T) {
	orch, _ := setupStateTestOrchestrator(t)

	// Save a pipeline to SQLite
	if err := orch.StartPipeline("recover-sqlite-001"); err != nil {
		t.Fatalf("StartPipeline failed: %v", err)
	}
	if err := orch.TransitionTo(StagePOConversation); err != nil {
		t.Fatalf("TransitionTo failed: %v", err)
	}
	orch.Pipeline.Agents["test-agent"] = AgentStatus{
		AgentID: "test-agent",
		Status:  AgentStatusRunning,
	}
	if err := orch.savePipelineState(); err != nil {
		t.Fatalf("savePipelineState failed: %v", err)
	}

	// Create a new orchestrator with the same store and recover
	orch2 := NewOrchestrator(orch.Config, orch.Store, orch.WorkDir)
	orch2.SetPipelineID("recover-sqlite-001")

	if err := orch2.Recover(); err != nil {
		t.Fatalf("Recover failed: %v", err)
	}
	if orch2.Pipeline == nil {
		t.Fatal("pipeline should be recovered from SQLite")
	}
	if orch2.Pipeline.ID != "recover-sqlite-001" {
		t.Errorf("pipeline ID = %s, want recover-sqlite-001", orch2.Pipeline.ID)
	}
	if orch2.Pipeline.CurrentStage != StagePOConversation {
		t.Errorf("stage = %s, want %s", orch2.Pipeline.CurrentStage, StagePOConversation)
	}
	if _, ok := orch2.Pipeline.Agents["test-agent"]; !ok {
		t.Error("agent status should be preserved after recovery")
	}
}

func TestRecover_NoState_SQLite(t *testing.T) {
	orch, _ := setupStateTestOrchestrator(t)

	// No pipeline in SQLite — Recover should return nil with no pipeline
	orch.SetPipelineID("nonexistent-001")
	if err := orch.Recover(); err != nil {
		t.Fatalf("Recover with no state should succeed: %v", err)
	}
	if orch.Pipeline != nil {
		t.Error("pipeline should remain nil when no state to recover")
	}
}

func TestRecover_ActivePipeline(t *testing.T) {
	orch, _ := setupStateTestOrchestrator(t)

	// Create multiple pipelines, some active, some terminal
	for _, tc := range []struct {
		id    string
		stage Stage
	}{
		{"old-completed", StageCompleted},
		{"old-failed", StageFailed},
		{"active-002", StagePOConversation},
		{"active-001", StageArchitectAnalysis},
	} {
		p := NewPipeline(tc.id, 3)
		p.CurrentStage = tc.stage
		data, _ := p.Snapshot()
		if err := orch.Store.UpsertPipeline(&store.PipelineRecord{
			PipelineID: tc.id,
			Stage:      string(tc.stage),
			StateJSON:  string(data),
			UpdatedAt:  time.Now(),
		}); err != nil {
			t.Fatalf("UpsertPipeline failed for %s: %v", tc.id, err)
		}
		// Small delay so updated_at order is deterministic
		time.Sleep(10 * time.Millisecond)
	}

	// Recover without specific ID — should get the most recently updated active pipeline
	orch2 := NewOrchestrator(orch.Config, orch.Store, orch.WorkDir)
	if err := orch2.Recover(); err != nil {
		t.Fatalf("Recover failed: %v", err)
	}
	if orch2.Pipeline == nil {
		t.Fatal("should recover an active pipeline")
	}
	// The most recently updated active pipeline is "active-001" (inserted last among active ones)
	if orch2.Pipeline.ID != "active-001" {
		t.Errorf("pipeline ID = %s, want active-001 (most recent active)", orch2.Pipeline.ID)
	}
}

func TestRecover_SpecificPipelineID(t *testing.T) {
	orch, _ := setupStateTestOrchestrator(t)

	// Create two active pipelines
	for _, id := range []string{"pipeline-a", "pipeline-b"} {
		p := NewPipeline(id, 3)
		p.CurrentStage = StagePOConversation
		data, _ := p.Snapshot()
		orch.Store.UpsertPipeline(&store.PipelineRecord{
			PipelineID: id,
			Stage:      string(StagePOConversation),
			StateJSON:  string(data),
			UpdatedAt:  time.Now(),
		})
		time.Sleep(10 * time.Millisecond)
	}

	// Recover specific pipeline-a (not the most recent)
	orch2 := NewOrchestrator(orch.Config, orch.Store, orch.WorkDir)
	orch2.SetPipelineID("pipeline-a")

	if err := orch2.Recover(); err != nil {
		t.Fatalf("Recover failed: %v", err)
	}
	if orch2.Pipeline == nil {
		t.Fatal("pipeline should be recovered")
	}
	if orch2.Pipeline.ID != "pipeline-a" {
		t.Errorf("pipeline ID = %s, want pipeline-a", orch2.Pipeline.ID)
	}
}

func TestTransitionTo_PersistsToSQLite(t *testing.T) {
	orch, _ := setupStateTestOrchestrator(t)

	if err := orch.StartPipeline("transition-001"); err != nil {
		t.Fatalf("StartPipeline failed: %v", err)
	}
	if err := orch.TransitionTo(StagePOConversation); err != nil {
		t.Fatalf("TransitionTo failed: %v", err)
	}

	// Verify SQLite reflects the transition
	rec, err := orch.Store.GetPipeline("transition-001")
	if err != nil {
		t.Fatalf("GetPipeline failed: %v", err)
	}
	if rec == nil {
		t.Fatal("pipeline should exist in SQLite")
	}
	if rec.Stage != string(StagePOConversation) {
		t.Errorf("stored stage = %s, want %s", rec.Stage, StagePOConversation)
	}

	// Also verify the full state can be deserialized
	p, err := LoadPipeline([]byte(rec.StateJSON))
	if err != nil {
		t.Fatalf("LoadPipeline failed: %v", err)
	}
	if len(p.History) != 1 {
		t.Errorf("history length = %d, want 1", len(p.History))
	}
	if p.History[0].From != StageInit || p.History[0].To != StagePOConversation {
		t.Errorf("history[0] = %s→%s, want %s→%s", p.History[0].From, p.History[0].To, StageInit, StagePOConversation)
	}
}

func TestForceStage_PersistsToSQLite(t *testing.T) {
	orch, _ := setupStateTestOrchestrator(t)

	if err := orch.StartPipeline("force-001"); err != nil {
		t.Fatalf("StartPipeline failed: %v", err)
	}

	// Force to a stage (bypassing validation)
	if err := orch.ForceStage(StageArchitectAnalysis); err != nil {
		t.Fatalf("ForceStage failed: %v", err)
	}

	// Verify SQLite reflects the forced stage
	rec, err := orch.Store.GetPipeline("force-001")
	if err != nil {
		t.Fatalf("GetPipeline failed: %v", err)
	}
	if rec == nil {
		t.Fatal("pipeline should exist in SQLite")
	}
	if rec.Stage != string(StageArchitectAnalysis) {
		t.Errorf("stored stage = %s, want %s", rec.Stage, StageArchitectAnalysis)
	}

	// Verify force transition is recorded in history
	p, err := LoadPipeline([]byte(rec.StateJSON))
	if err != nil {
		t.Fatalf("LoadPipeline failed: %v", err)
	}
	if len(p.History) != 1 {
		t.Fatalf("history length = %d, want 1", len(p.History))
	}
	if p.History[0].From != StageInit || p.History[0].To != StageArchitectAnalysis {
		t.Errorf("history[0] = %s→%s, want %s→%s",
			p.History[0].From, p.History[0].To, StageInit, StageArchitectAnalysis)
	}
}

func TestStartPipeline_PersistsToSQLite(t *testing.T) {
	orch, _ := setupStateTestOrchestrator(t)

	if err := orch.StartPipeline("start-persist-001"); err != nil {
		t.Fatalf("StartPipeline failed: %v", err)
	}

	// Verify SQLite has the pipeline with correct initial state
	rec, err := orch.Store.GetPipeline("start-persist-001")
	if err != nil {
		t.Fatalf("GetPipeline failed: %v", err)
	}
	if rec == nil {
		t.Fatal("pipeline should exist in SQLite after StartPipeline")
	}
	if rec.PipelineID != "start-persist-001" {
		t.Errorf("pipeline ID = %s, want start-persist-001", rec.PipelineID)
	}
	if rec.Stage != string(StageInit) {
		t.Errorf("stage = %s, want %s", rec.Stage, StageInit)
	}

	// Verify the full state JSON is valid
	p, err := LoadPipeline([]byte(rec.StateJSON))
	if err != nil {
		t.Fatalf("LoadPipeline failed: %v", err)
	}
	if p.ID != "start-persist-001" {
		t.Errorf("deserialized ID = %s, want start-persist-001", p.ID)
	}
	if p.CurrentStage != StageInit {
		t.Errorf("deserialized stage = %s, want %s", p.CurrentStage, StageInit)
	}
	if p.MaxAttempts != 3 {
		t.Errorf("maxAttempts = %d, want 3", p.MaxAttempts)
	}
}
