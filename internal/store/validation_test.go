package store

import (
	"errors"
	"math"
	"testing"
)

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
