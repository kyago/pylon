package orchestrator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConversationManager_Create(t *testing.T) {
	dir := t.TempDir()
	mgr := NewConversationManager(dir)

	conv, err := mgr.Create("conv-001", "테스트 대화")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if conv.ID != "conv-001" {
		t.Errorf("ID = %s, want conv-001", conv.ID)
	}
	if conv.Meta.Status != "active" {
		t.Errorf("status = %s, want active", conv.Meta.Status)
	}

	// Check meta.yml exists
	metaPath := filepath.Join(dir, "conv-001", "meta.yml")
	if _, err := os.Stat(metaPath); os.IsNotExist(err) {
		t.Error("meta.yml should exist")
	}

	// Check thread.md exists with header
	threadPath := filepath.Join(dir, "conv-001", "thread.md")
	data, err := os.ReadFile(threadPath)
	if err != nil {
		t.Fatalf("failed to read thread.md: %v", err)
	}
	if !strings.Contains(string(data), "테스트 대화") {
		t.Error("thread.md should contain conversation title")
	}
}

func TestConversationManager_AppendMessage(t *testing.T) {
	dir := t.TempDir()
	mgr := NewConversationManager(dir)

	mgr.Create("conv-002", "메시지 테스트")

	if err := mgr.AppendMessage("conv-002", "user", "안녕하세요"); err != nil {
		t.Fatalf("AppendMessage failed: %v", err)
	}
	if err := mgr.AppendMessage("conv-002", "assistant", "반갑습니다"); err != nil {
		t.Fatalf("AppendMessage failed: %v", err)
	}

	threadPath := filepath.Join(dir, "conv-002", "thread.md")
	data, err := os.ReadFile(threadPath)
	if err != nil {
		t.Fatalf("failed to read thread.md: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "user") {
		t.Error("thread should contain user role")
	}
	if !strings.Contains(content, "안녕하세요") {
		t.Error("thread should contain user message")
	}
	if !strings.Contains(content, "assistant") {
		t.Error("thread should contain assistant role")
	}
	if !strings.Contains(content, "반갑습니다") {
		t.Error("thread should contain assistant message")
	}
}

func TestConversationManager_Load(t *testing.T) {
	dir := t.TempDir()
	mgr := NewConversationManager(dir)

	mgr.Create("conv-003", "로드 테스트")

	conv, err := mgr.Load("conv-003")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if conv.ID != "conv-003" {
		t.Errorf("ID = %s, want conv-003", conv.ID)
	}
	if conv.Meta.Status != "active" {
		t.Errorf("status = %s, want active", conv.Meta.Status)
	}
}

func TestConversationManager_Load_NotFound(t *testing.T) {
	dir := t.TempDir()
	mgr := NewConversationManager(dir)

	_, err := mgr.Load("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent conversation")
	}
}

func TestComputeAmbiguity_Greenfield_AllClear(t *testing.T) {
	scores := ClarityScores{Goal: 1.0, Constraints: 1.0, Criteria: 1.0}
	got := ComputeAmbiguity(scores, false)
	if got != 0.0 {
		t.Errorf("all clear greenfield: got %f, want 0.0", got)
	}
}

func TestComputeAmbiguity_Greenfield_AllUnclear(t *testing.T) {
	scores := ClarityScores{Goal: 0.0, Constraints: 0.0, Criteria: 0.0}
	got := ComputeAmbiguity(scores, false)
	if got != 1.0 {
		t.Errorf("all unclear greenfield: got %f, want 1.0", got)
	}
}

func TestComputeAmbiguity_Greenfield_Partial(t *testing.T) {
	scores := ClarityScores{Goal: 0.8, Constraints: 0.6, Criteria: 0.4}
	got := ComputeAmbiguity(scores, false)
	// 1 - (0.8*0.40 + 0.6*0.30 + 0.4*0.30) = 1 - (0.32 + 0.18 + 0.12) = 1 - 0.62 = 0.38
	want := 0.38
	if diff := got - want; diff > 0.001 || diff < -0.001 {
		t.Errorf("partial greenfield: got %f, want %f", got, want)
	}
}

func TestComputeAmbiguity_Brownfield_AllClear(t *testing.T) {
	scores := ClarityScores{Goal: 1.0, Constraints: 1.0, Criteria: 1.0, Context: 1.0}
	got := ComputeAmbiguity(scores, true)
	if got != 0.0 {
		t.Errorf("all clear brownfield: got %f, want 0.0", got)
	}
}

func TestComputeAmbiguity_Brownfield_Partial(t *testing.T) {
	scores := ClarityScores{Goal: 0.8, Constraints: 0.6, Criteria: 0.4, Context: 0.5}
	got := ComputeAmbiguity(scores, true)
	// 1 - (0.8*0.35 + 0.6*0.25 + 0.4*0.25 + 0.5*0.15) = 1 - (0.28 + 0.15 + 0.10 + 0.075) = 1 - 0.605 = 0.395
	want := 0.395
	if diff := got - want; diff > 0.001 || diff < -0.001 {
		t.Errorf("partial brownfield: got %f, want %f", got, want)
	}
}

func TestComputeAmbiguity_Clamps(t *testing.T) {
	// Values outside [0,1] should be clamped
	scores := ClarityScores{Goal: 1.5, Constraints: -0.2, Criteria: 0.5}
	got := ComputeAmbiguity(scores, false)
	// clamped: goal=1.0, constraints=0.0, criteria=0.5
	// 1 - (1.0*0.40 + 0.0*0.30 + 0.5*0.30) = 1 - (0.40 + 0.00 + 0.15) = 0.45
	want := 0.45
	if diff := got - want; diff > 0.001 || diff < -0.001 {
		t.Errorf("clamped: got %f, want %f", got, want)
	}
}

func TestIsReadyForExecution(t *testing.T) {
	scores := &ClarityScores{Goal: 1.0, Constraints: 1.0, Criteria: 1.0}

	tests := []struct {
		name      string
		ambiguity float64
		scores    *ClarityScores
		threshold float64
		want      bool
	}{
		{"clear enough", 0.2, scores, 0.3, true},
		{"exactly at threshold", 0.3, scores, 0.3, true},
		{"too ambiguous", 0.5, scores, 0.3, false},
		{"zero ambiguity", 0.0, scores, 0.3, true},
		{"nil clarity scores", 0.0, nil, 0.3, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conv := &Conversation{
				Meta: ConversationMeta{
					AmbiguityScore: tt.ambiguity,
					ClarityScores:  tt.scores,
				},
			}
			got := conv.IsReadyForExecution(tt.threshold)
			if got != tt.want {
				t.Errorf("IsReadyForExecution(%f): got %v, want %v", tt.threshold, got, tt.want)
			}
		})
	}
}

func TestConversationManager_SaveMeta_WithAmbiguity(t *testing.T) {
	dir := t.TempDir()
	mgr := NewConversationManager(dir)

	mgr.Create("conv-amb", "모호성 테스트")

	meta := ConversationMeta{
		Status:         "active",
		AmbiguityScore: 0.35,
		ClarityScores: &ClarityScores{
			Goal:        0.8,
			Constraints: 0.6,
			Criteria:    0.5,
		},
	}
	if err := mgr.SaveMeta("conv-amb", meta); err != nil {
		t.Fatalf("SaveMeta failed: %v", err)
	}

	conv, err := mgr.Load("conv-amb")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if conv.Meta.AmbiguityScore != 0.35 {
		t.Errorf("ambiguity_score = %f, want 0.35", conv.Meta.AmbiguityScore)
	}
	if conv.Meta.ClarityScores == nil {
		t.Fatal("clarity_scores should not be nil")
	}
	if conv.Meta.ClarityScores.Goal != 0.8 {
		t.Errorf("goal = %f, want 0.8", conv.Meta.ClarityScores.Goal)
	}
}

func TestConversationManager_SaveMeta(t *testing.T) {
	dir := t.TempDir()
	mgr := NewConversationManager(dir)

	mgr.Create("conv-004", "메타 업데이트")

	newMeta := ConversationMeta{
		Status:   "completed",
		TaskID:   "task-123",
		Projects: []string{"proj-a"},
	}
	if err := mgr.SaveMeta("conv-004", newMeta); err != nil {
		t.Fatalf("SaveMeta failed: %v", err)
	}

	conv, err := mgr.Load("conv-004")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if conv.Meta.Status != "completed" {
		t.Errorf("status = %s, want completed", conv.Meta.Status)
	}
	if conv.Meta.TaskID != "task-123" {
		t.Errorf("taskID = %s, want task-123", conv.Meta.TaskID)
	}
}
