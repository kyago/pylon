package store

import (
	"errors"
	"fmt"
	"math"
	"testing"
	"time"
)

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
		"init", "po_conversation", "architect_analysis",
		"pm_task_breakdown", "agent_executing", "verification",
		"pr_creation", "po_validation", "wiki_update",
		"completed", "failed",
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
		{"NaN", math.NaN(), true},
		{"positive Inf", math.Inf(1), true},
		{"negative Inf", math.Inf(-1), true},
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
		{"po_conversation", "po_conversation", false},
		{"architect_analysis", "architect_analysis", false},
		{"pm_task_breakdown", "pm_task_breakdown", false},
		{"agent_executing", "agent_executing", false},
		{"verification", "verification", false},
		{"pr_creation", "pr_creation", false},
		{"po_validation", "po_validation", false},
		{"wiki_update", "wiki_update", false},
		{"completed", "completed", false},
		{"failed", "failed", false},
		{"invalid", "bogus", true},
		{"empty", "", true},
		{"uppercase", "INIT", true},
		{"removed analyzing", "analyzing", true},
		{"removed planning", "planning", true},
		{"removed executing", "executing", true},
		{"removed verifying", "verifying", true},
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
