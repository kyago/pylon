package orchestrator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kyago/pylon/internal/store"
)

func newConvTestStore(t *testing.T) *store.Store {
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

func TestConversationManager_Create(t *testing.T) {
	dir := t.TempDir()
	s := newConvTestStore(t)
	mgr := NewConversationManager(dir, s)

	conv, err := mgr.Create("conv-001", "테스트 대화")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if conv.ID != "conv-001" {
		t.Errorf("ID = %s, want conv-001", conv.ID)
	}
	if conv.Meta.Status != ConvStatusActive {
		t.Errorf("status = %s, want active", conv.Meta.Status)
	}

	// Check conversation in Store
	rec, err := s.GetConversation("conv-001")
	if err != nil {
		t.Fatalf("GetConversation failed: %v", err)
	}
	if rec == nil {
		t.Fatal("conversation should exist in store")
	}
	if rec.Status != ConvStatusActive {
		t.Errorf("store status = %s, want active", rec.Status)
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
	s := newConvTestStore(t)
	mgr := NewConversationManager(dir, s)

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
	s := newConvTestStore(t)
	mgr := NewConversationManager(dir, s)

	mgr.Create("conv-003", "로드 테스트")

	conv, err := mgr.Load("conv-003")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if conv.ID != "conv-003" {
		t.Errorf("ID = %s, want conv-003", conv.ID)
	}
	if conv.Meta.Status != ConvStatusActive {
		t.Errorf("status = %s, want active", conv.Meta.Status)
	}
}

func TestConversationManager_Load_NotFound(t *testing.T) {
	dir := t.TempDir()
	s := newConvTestStore(t)
	mgr := NewConversationManager(dir, s)

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
	s := newConvTestStore(t)
	mgr := NewConversationManager(dir, s)

	mgr.Create("conv-amb", "모호성 테스트")

	meta := ConversationMeta{
		Status:         ConvStatusActive,
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

func TestConversationMeta_CompletedAt_SessionID_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	s := newConvTestStore(t)
	mgr := NewConversationManager(dir, s)

	mgr.Create("conv-rt", "라운드트립 테스트")

	meta := ConversationMeta{
		Status:      ConvStatusCompleted,
		StartedAt:   "2026-03-13T10:00:00+09:00",
		CompletedAt: "2026-03-13T11:00:00+09:00",
		SessionID:   "session-abc-123",
		TaskID:      "task-456",
		Projects:    []string{"proj-a"},
	}
	if err := mgr.SaveMeta("conv-rt", meta); err != nil {
		t.Fatalf("SaveMeta failed: %v", err)
	}

	conv, err := mgr.Load("conv-rt")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if conv.Meta.CompletedAt == "" {
		t.Error("completed_at should not be empty")
	}
	if conv.Meta.SessionID != "session-abc-123" {
		t.Errorf("session_id = %q, want %q", conv.Meta.SessionID, "session-abc-123")
	}
}

func TestConversationMeta_OmitEmpty(t *testing.T) {
	dir := t.TempDir()
	s := newConvTestStore(t)
	mgr := NewConversationManager(dir, s)

	mgr.Create("conv-omit", "빈 필드 테스트")

	// Save without CompletedAt and SessionID
	meta := ConversationMeta{
		Status:    ConvStatusActive,
		StartedAt: "2026-03-13T10:00:00+09:00",
	}
	if err := mgr.SaveMeta("conv-omit", meta); err != nil {
		t.Fatalf("SaveMeta failed: %v", err)
	}

	conv, err := mgr.Load("conv-omit")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if conv.Meta.CompletedAt != "" {
		t.Errorf("completed_at should be empty, got %q", conv.Meta.CompletedAt)
	}
	if conv.Meta.SessionID != "" {
		t.Errorf("session_id should be empty, got %q", conv.Meta.SessionID)
	}
}

func TestConversationManager_SaveMeta(t *testing.T) {
	dir := t.TempDir()
	s := newConvTestStore(t)
	mgr := NewConversationManager(dir, s)

	mgr.Create("conv-004", "메타 업데이트")

	newMeta := ConversationMeta{
		Status:   ConvStatusCompleted,
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
	if conv.Meta.Status != ConvStatusCompleted {
		t.Errorf("status = %s, want completed", conv.Meta.Status)
	}
	if conv.Meta.TaskID != "task-123" {
		t.Errorf("taskID = %s, want task-123", conv.Meta.TaskID)
	}
}

func TestConversationManager_Complete(t *testing.T) {
	dir := t.TempDir()
	s := newConvTestStore(t)
	mgr := NewConversationManager(dir, s)

	mgr.Create("conv-complete", "완료 테스트")

	if err := mgr.Complete("conv-complete"); err != nil {
		t.Fatalf("Complete failed: %v", err)
	}

	conv, err := mgr.Load("conv-complete")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if conv.Meta.Status != ConvStatusCompleted {
		t.Errorf("status = %s, want completed", conv.Meta.Status)
	}
	if conv.Meta.CompletedAt == "" {
		t.Error("completed_at should be set")
	}
}

func TestConversationManager_List(t *testing.T) {
	dir := t.TempDir()
	s := newConvTestStore(t)
	mgr := NewConversationManager(dir, s)

	mgr.Create("conv-list-1", "대화 1")
	mgr.Create("conv-list-2", "대화 2")
	mgr.Complete("conv-list-2")

	// List all
	all, err := mgr.List("")
	if err != nil {
		t.Fatalf("List all failed: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 conversations, got %d", len(all))
	}

	// List active only
	active, err := mgr.List("active")
	if err != nil {
		t.Fatalf("List active failed: %v", err)
	}
	if len(active) != 1 {
		t.Errorf("expected 1 active conversation, got %d", len(active))
	}

	// List completed only
	completed, err := mgr.List("completed")
	if err != nil {
		t.Fatalf("List completed failed: %v", err)
	}
	if len(completed) != 1 {
		t.Errorf("expected 1 completed conversation, got %d", len(completed))
	}
}
