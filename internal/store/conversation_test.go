package store

import (
	"testing"
	"time"
)

func TestConversation_UpsertGet(t *testing.T) {
	s := setupTestStore(t)

	now := time.Now().Truncate(time.Second)
	rec := &ConversationRecord{
		ID:             "conv-001",
		Title:          "테스트 대화",
		Status:         "active",
		SessionID:      "sess-123",
		PipelineID:     "pipe-001",
		Projects:       "proj-a,proj-b",
		TaskID:         "task-001",
		AmbiguityScore: 0.35,
		ClarityScores:  `{"goal":0.8,"constraints":0.6,"criteria":0.5}`,
		StartedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.UpsertConversation(rec); err != nil {
		t.Fatalf("UpsertConversation failed: %v", err)
	}

	got, err := s.GetConversation("conv-001")
	if err != nil {
		t.Fatalf("GetConversation failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected conversation, got nil")
	}
	if got.Title != "테스트 대화" {
		t.Errorf("title = %q, want %q", got.Title, "테스트 대화")
	}
	if got.Status != "active" {
		t.Errorf("status = %q, want active", got.Status)
	}
	if got.SessionID != "sess-123" {
		t.Errorf("session_id = %q, want sess-123", got.SessionID)
	}
	if got.PipelineID != "pipe-001" {
		t.Errorf("pipeline_id = %q, want pipe-001", got.PipelineID)
	}
	if got.Projects != "proj-a,proj-b" {
		t.Errorf("projects = %q, want proj-a,proj-b", got.Projects)
	}
	if got.AmbiguityScore != 0.35 {
		t.Errorf("ambiguity_score = %f, want 0.35", got.AmbiguityScore)
	}

	// Update via upsert
	rec.Status = "completed"
	completedAt := now.Add(time.Hour)
	rec.CompletedAt = &completedAt
	rec.UpdatedAt = time.Now()
	if err := s.UpsertConversation(rec); err != nil {
		t.Fatalf("UpsertConversation (update) failed: %v", err)
	}

	got2, err := s.GetConversation("conv-001")
	if err != nil {
		t.Fatalf("GetConversation after update failed: %v", err)
	}
	if got2.Status != "completed" {
		t.Errorf("status after update = %q, want completed", got2.Status)
	}
	if got2.CompletedAt == nil {
		t.Error("completed_at should not be nil")
	}
}

func TestConversation_GetNotFound(t *testing.T) {
	s := setupTestStore(t)

	got, err := s.GetConversation("nonexistent")
	if err != nil {
		t.Fatalf("GetConversation failed: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for nonexistent, got %+v", got)
	}
}

func TestConversation_ListByStatus(t *testing.T) {
	s := setupTestStore(t)
	now := time.Now()

	recs := []ConversationRecord{
		{ID: "c1", Title: "Active 1", Status: "active", StartedAt: now, UpdatedAt: now},
		{ID: "c2", Title: "Completed", Status: "completed", StartedAt: now, UpdatedAt: now},
		{ID: "c3", Title: "Active 2", Status: "active", StartedAt: now, UpdatedAt: now},
		{ID: "c4", Title: "Cancelled", Status: "cancelled", StartedAt: now, UpdatedAt: now},
	}
	for i := range recs {
		if err := s.UpsertConversation(&recs[i]); err != nil {
			t.Fatalf("UpsertConversation failed: %v", err)
		}
	}

	// All
	all, err := s.ListConversations("")
	if err != nil {
		t.Fatalf("ListConversations all failed: %v", err)
	}
	if len(all) != 4 {
		t.Errorf("expected 4, got %d", len(all))
	}

	// Active
	active, err := s.ListConversations("active")
	if err != nil {
		t.Fatalf("ListConversations active failed: %v", err)
	}
	if len(active) != 2 {
		t.Errorf("expected 2 active, got %d", len(active))
	}

	// Completed
	completed, err := s.ListConversations("completed")
	if err != nil {
		t.Fatalf("ListConversations completed failed: %v", err)
	}
	if len(completed) != 1 {
		t.Errorf("expected 1 completed, got %d", len(completed))
	}

	// Cancelled
	cancelled, err := s.ListConversations("cancelled")
	if err != nil {
		t.Fatalf("ListConversations cancelled failed: %v", err)
	}
	if len(cancelled) != 1 {
		t.Errorf("expected 1 cancelled, got %d", len(cancelled))
	}
}

func TestConversation_UpdateStatus(t *testing.T) {
	s := setupTestStore(t)
	now := time.Now()

	rec := &ConversationRecord{
		ID:        "c-update",
		Title:     "Status Test",
		Status:    "active",
		StartedAt: now,
		UpdatedAt: now,
	}
	if err := s.UpsertConversation(rec); err != nil {
		t.Fatalf("UpsertConversation failed: %v", err)
	}

	completedAt := now.Add(time.Hour)
	if err := s.UpdateConversationStatus("c-update", "completed", &completedAt); err != nil {
		t.Fatalf("UpdateConversationStatus failed: %v", err)
	}

	got, _ := s.GetConversation("c-update")
	if got.Status != "completed" {
		t.Errorf("status = %q, want completed", got.Status)
	}
	if got.CompletedAt == nil {
		t.Error("completed_at should not be nil")
	}
}

func TestConversation_UpdateStatus_NotFound(t *testing.T) {
	s := setupTestStore(t)

	err := s.UpdateConversationStatus("nonexistent", "completed", nil)
	if err == nil {
		t.Error("expected error for nonexistent conversation")
	}
}

func TestConversation_UpdateStatus_InvalidStatus(t *testing.T) {
	s := setupTestStore(t)
	now := time.Now()

	rec := &ConversationRecord{
		ID:        "c-invalid",
		Title:     "Invalid Status",
		Status:    "active",
		StartedAt: now,
		UpdatedAt: now,
	}
	s.UpsertConversation(rec)

	err := s.UpdateConversationStatus("c-invalid", "bogus", nil)
	if err == nil {
		t.Error("expected error for invalid status")
	}
}

func TestConversation_ClarityScoresJSON(t *testing.T) {
	// Marshal
	scores := map[string]float64{
		"goal":        0.8,
		"constraints": 0.6,
		"criteria":    0.5,
	}
	json := MarshalClarityScores(scores)
	if json == "" {
		t.Fatal("MarshalClarityScores returned empty")
	}

	// Unmarshal
	got := UnmarshalClarityScores(json)
	if got == nil {
		t.Fatal("UnmarshalClarityScores returned nil")
	}
	if got["goal"] != 0.8 {
		t.Errorf("goal = %f, want 0.8", got["goal"])
	}
	if got["constraints"] != 0.6 {
		t.Errorf("constraints = %f, want 0.6", got["constraints"])
	}
	if got["criteria"] != 0.5 {
		t.Errorf("criteria = %f, want 0.5", got["criteria"])
	}

	// Empty roundtrip
	emptyJSON := MarshalClarityScores(nil)
	if emptyJSON != "" {
		t.Errorf("expected empty for nil, got %q", emptyJSON)
	}
	emptyResult := UnmarshalClarityScores("")
	if emptyResult != nil {
		t.Errorf("expected nil for empty, got %v", emptyResult)
	}
}

func TestConversation_UpsertInvalidStatus(t *testing.T) {
	s := setupTestStore(t)

	rec := &ConversationRecord{
		ID:        "c-bad",
		Title:     "Bad Status",
		Status:    "invalid",
		StartedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := s.UpsertConversation(rec)
	if err == nil {
		t.Error("expected error for invalid status")
	}
}

