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
	ErrInvalidMessageStatus = errors.New("invalid message queue status")
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

var validMessageStatuses = map[string]bool{
	"queued":    true,
	"delivered": true,
	"acked":     true,
	"expired":   true,
	"failed":    true,
}

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

// validateMessageStatus checks that a status is one of the valid message statuses.
func validateMessageStatus(status string) error {
	if !validMessageStatuses[status] {
		return fmt.Errorf("%w: %q", ErrInvalidMessageStatus, status)
	}
	return nil
}
