package store

import (
	"errors"
	"fmt"
	"math"

	"github.com/kyago/pylon/internal/domain"
)

// Validation errors returned when input values violate constraints.
var (
	ErrConfidenceOutOfRange = errors.New("confidence must be between 0.0 and 1.0")
	ErrInvalidPipelineStage = errors.New("invalid pipeline stage")
)

// validPipelineStages is built from domain.AllStages() to guarantee
// compile-time synchronization with the Stage constants.
var validPipelineStages = func() map[string]bool {
	m := make(map[string]bool)
	for _, s := range domain.AllStages() {
		m[string(s)] = true
	}
	return m
}()

// validateConfidence checks that a confidence value is within [0.0, 1.0].
func validateConfidence(confidence float64) error {
	if math.IsNaN(confidence) || math.IsInf(confidence, 0) {
		return fmt.Errorf("%w: got %v", ErrConfidenceOutOfRange, confidence)
	}
	if confidence < 0.0 || confidence > 1.0 {
		return fmt.Errorf("%w: got %v", ErrConfidenceOutOfRange, confidence)
	}
	return nil
}

// validatePipelineStage checks that a stage is one of the valid pipeline stages.
func validatePipelineStage(stage string) error {
	if !validPipelineStages[stage] {
		return fmt.Errorf("%w: %q", ErrInvalidPipelineStage, stage)
	}
	return nil
}

// ErrInvalidConversationStatus is returned when a conversation status value is not recognized.
var ErrInvalidConversationStatus = errors.New("invalid conversation status")

var validConversationStatuses = map[string]bool{
	"active":    true,
	"completed": true,
	"cancelled": true,
}

// validateConversationStatus checks that a status is one of the valid conversation statuses.
func validateConversationStatus(status string) error {
	if !validConversationStatuses[status] {
		return fmt.Errorf("%w: %q", ErrInvalidConversationStatus, status)
	}
	return nil
}
