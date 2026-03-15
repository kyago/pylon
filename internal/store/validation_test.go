package store

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

// --- Blackboard confidence validation ---

func TestBlackboard_RejectsNegativeConfidence(t *testing.T) {
	s := setupTestStore(t)

	entry := &BlackboardEntry{
		ProjectID:  "proj-1",
		Category:   "decision",
		Key:        "k1",
		Value:      `"v1"`,
		Confidence: -0.1,
		Author:     "agent",
	}

	err := s.PutBlackboard(entry)
	if err == nil {
		t.Fatal("expected error for negative confidence")
	}
	if !errors.Is(err, ErrConfidenceOutOfRange) {
		t.Errorf("expected ErrConfidenceOutOfRange, got: %v", err)
	}
}

func TestBlackboard_RejectsConfidenceAboveOne(t *testing.T) {
	s := setupTestStore(t)

	entry := &BlackboardEntry{
		ProjectID:  "proj-1",
		Category:   "decision",
		Key:        "k1",
		Value:      `"v1"`,
		Confidence: 1.1,
		Author:     "agent",
	}

	err := s.PutBlackboard(entry)
	if err == nil {
		t.Fatal("expected error for confidence > 1.0")
	}
	if !errors.Is(err, ErrConfidenceOutOfRange) {
		t.Errorf("expected ErrConfidenceOutOfRange, got: %v", err)
	}
}

func TestBlackboard_AcceptsBoundaryConfidence(t *testing.T) {
	s := setupTestStore(t)

	// 0.0 should be accepted
	err := s.PutBlackboard(&BlackboardEntry{
		ProjectID:  "proj-1",
		Category:   "decision",
		Key:        "k-zero",
		Value:      `"v"`,
		Confidence: 0.0,
		Author:     "agent",
	})
	if err != nil {
		t.Errorf("confidence 0.0 should be valid, got error: %v", err)
	}

	// 1.0 should be accepted
	err = s.PutBlackboard(&BlackboardEntry{
		ProjectID:  "proj-1",
		Category:   "decision",
		Key:        "k-one",
		Value:      `"v"`,
		Confidence: 1.0,
		Author:     "agent",
	})
	if err != nil {
		t.Errorf("confidence 1.0 should be valid, got error: %v", err)
	}
}

// --- Pipeline stage validation ---

func TestPipeline_RejectsInvalidStage(t *testing.T) {
	s := setupTestStore(t)

	rec := &PipelineRecord{
		PipelineID: "pipe-1",
		Stage:      "invalid_stage",
		StateJSON:  `{}`,
		UpdatedAt:  time.Now(),
	}

	err := s.UpsertPipeline(rec)
	if err == nil {
		t.Fatal("expected error for invalid stage")
	}
	if !errors.Is(err, ErrInvalidPipelineStage) {
		t.Errorf("expected ErrInvalidPipelineStage, got: %v", err)
	}
}

func TestPipeline_AcceptsAllValidStages(t *testing.T) {
	s := setupTestStore(t)

	validStages := []string{
		"init", "analyzing", "planning", "executing",
		"verifying", "completed", "failed",
		"po_conversation", "agent_executing",
	}

	for i, stage := range validStages {
		rec := &PipelineRecord{
			PipelineID: fmt.Sprintf("pipe-%d", i),
			Stage:      stage,
			StateJSON:  `{}`,
			UpdatedAt:  time.Now(),
		}
		if err := s.UpsertPipeline(rec); err != nil {
			t.Errorf("stage %q should be valid, got error: %v", stage, err)
		}
	}
}

func TestPipeline_RejectsEmptyStage(t *testing.T) {
	s := setupTestStore(t)

	rec := &PipelineRecord{
		PipelineID: "pipe-empty",
		Stage:      "",
		StateJSON:  `{}`,
		UpdatedAt:  time.Now(),
	}

	err := s.UpsertPipeline(rec)
	if err == nil {
		t.Fatal("expected error for empty stage")
	}
	if !errors.Is(err, ErrInvalidPipelineStage) {
		t.Errorf("expected ErrInvalidPipelineStage, got: %v", err)
	}
}

// --- Message queue status validation ---

func TestMessageQueue_RejectsInvalidStatus(t *testing.T) {
	s := setupTestStore(t)

	msg := &QueuedMessage{
		Type:      "task_assign",
		FromAgent: "orc",
		ToAgent:   "dev",
		Body:      `{}`,
		Status:    "invalid_status",
	}

	err := s.Enqueue(msg)
	if err == nil {
		t.Fatal("expected error for invalid status")
	}
	if !errors.Is(err, ErrInvalidMessageStatus) {
		t.Errorf("expected ErrInvalidMessageStatus, got: %v", err)
	}
}

func TestMessageQueue_AcceptsAllValidStatuses(t *testing.T) {
	s := setupTestStore(t)

	validStatuses := []string{"queued", "delivered", "acked", "expired", "failed"}

	for i, status := range validStatuses {
		msg := &QueuedMessage{
			Type:      "task_assign",
			FromAgent: "orc",
			ToAgent:   "dev",
			Body:      `{}`,
			Status:    status,
		}
		msg.ID = fmt.Sprintf("msg-%d", i)
		if err := s.Enqueue(msg); err != nil {
			t.Errorf("status %q should be valid, got error: %v", status, err)
		}
	}
}

func TestMessageQueue_DefaultStatusIsValid(t *testing.T) {
	s := setupTestStore(t)

	msg := &QueuedMessage{
		Type:      "task_assign",
		FromAgent: "orc",
		ToAgent:   "dev",
		Body:      `{}`,
		// Status is empty, should default to "queued"
	}

	err := s.Enqueue(msg)
	if err != nil {
		t.Errorf("default status should be valid, got error: %v", err)
	}
}

// --- Project memory confidence validation ---

func TestProjectMemory_RejectsNegativeConfidence(t *testing.T) {
	s := setupTestStore(t)

	entry := &MemoryEntry{
		ProjectID:  "proj-1",
		Category:   "learning",
		Key:        "k1",
		Content:    "content",
		Confidence: -0.5,
	}

	err := s.InsertMemory(entry)
	if err == nil {
		t.Fatal("expected error for negative confidence")
	}
	if !errors.Is(err, ErrConfidenceOutOfRange) {
		t.Errorf("expected ErrConfidenceOutOfRange, got: %v", err)
	}
}

func TestProjectMemory_RejectsConfidenceAboveOne(t *testing.T) {
	s := setupTestStore(t)

	entry := &MemoryEntry{
		ProjectID:  "proj-1",
		Category:   "learning",
		Key:        "k1",
		Content:    "content",
		Confidence: 2.0,
	}

	err := s.InsertMemory(entry)
	if err == nil {
		t.Fatal("expected error for confidence > 1.0")
	}
	if !errors.Is(err, ErrConfidenceOutOfRange) {
		t.Errorf("expected ErrConfidenceOutOfRange, got: %v", err)
	}
}

func TestProjectMemory_AcceptsBoundaryConfidence(t *testing.T) {
	s := setupTestStore(t)

	// 0.0 should be accepted
	err := s.InsertMemory(&MemoryEntry{
		ProjectID:  "proj-1",
		Category:   "learning",
		Key:        "k-zero",
		Content:    "content",
		Confidence: 0.0,
	})
	if err != nil {
		t.Errorf("confidence 0.0 should be valid, got error: %v", err)
	}

	// 1.0 should be accepted
	err = s.InsertMemory(&MemoryEntry{
		ProjectID:  "proj-1",
		Category:   "learning",
		Key:        "k-one",
		Content:    "content",
		Confidence: 1.0,
	})
	if err != nil {
		t.Errorf("confidence 1.0 should be valid, got error: %v", err)
	}
}

// --- Unit tests for validation functions ---

func TestValidateConfidence(t *testing.T) {
	tests := []struct {
		name       string
		confidence float64
		wantErr    bool
	}{
		{"zero", 0.0, false},
		{"mid", 0.5, false},
		{"one", 1.0, false},
		{"negative", -0.01, true},
		{"above one", 1.01, true},
		{"large negative", -100.0, true},
		{"large positive", 100.0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfidence(tt.confidence)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateConfidence(%v) error = %v, wantErr %v", tt.confidence, err, tt.wantErr)
			}
		})
	}
}

func TestValidatePipelineStage(t *testing.T) {
	tests := []struct {
		name    string
		stage   string
		wantErr bool
	}{
		{"init", "init", false},
		{"analyzing", "analyzing", false},
		{"planning", "planning", false},
		{"executing", "executing", false},
		{"verifying", "verifying", false},
		{"completed", "completed", false},
		{"failed", "failed", false},
		{"po_conversation", "po_conversation", false},
		{"agent_executing", "agent_executing", false},
		{"invalid", "bogus", true},
		{"empty", "", true},
		{"uppercase", "INIT", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePipelineStage(tt.stage)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePipelineStage(%q) error = %v, wantErr %v", tt.stage, err, tt.wantErr)
			}
		})
	}
}

func TestValidateMessageStatus(t *testing.T) {
	tests := []struct {
		name    string
		status  string
		wantErr bool
	}{
		{"queued", "queued", false},
		{"delivered", "delivered", false},
		{"acked", "acked", false},
		{"expired", "expired", false},
		{"failed", "failed", false},
		{"invalid", "bogus", true},
		{"empty", "", true},
		{"uppercase", "QUEUED", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMessageStatus(tt.status)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateMessageStatus(%q) error = %v, wantErr %v", tt.status, err, tt.wantErr)
			}
		})
	}
}
