package store

import (
	"errors"
	"fmt"
)

// Validation errors returned when input values violate constraints.
var (
	ErrConfidenceOutOfRange = errors.New("confidence must be between 0.0 and 1.0")
	ErrInvalidPipelineStage = errors.New("invalid pipeline stage")
	ErrInvalidMessageStatus = errors.New("invalid message queue status")
)

// ValidPipelineStages defines the allowed pipeline stages.
var ValidPipelineStages = map[string]bool{
	"init":             true,
	"analyzing":        true,
	"planning":         true,
	"executing":        true,
	"verifying":        true,
	"completed":        true,
	"failed":           true,
	"po_conversation":  true,
	"agent_executing":  true,
}

// ValidMessageStatuses defines the allowed message queue statuses.
var ValidMessageStatuses = map[string]bool{
	"queued":    true,
	"delivered": true,
	"acked":     true,
	"expired":   true,
	"failed":    true,
}

// validateConfidence checks that a confidence value is within [0.0, 1.0].
func validateConfidence(confidence float64) error {
	if confidence < 0.0 || confidence > 1.0 {
		return fmt.Errorf("%w: got %v", ErrConfidenceOutOfRange, confidence)
	}
	return nil
}

// validatePipelineStage checks that a stage is one of the valid pipeline stages.
func validatePipelineStage(stage string) error {
	if !ValidPipelineStages[stage] {
		return fmt.Errorf("%w: %q", ErrInvalidPipelineStage, stage)
	}
	return nil
}

// validateMessageStatus checks that a status is one of the valid message statuses.
func validateMessageStatus(status string) error {
	if !ValidMessageStatuses[status] {
		return fmt.Errorf("%w: %q", ErrInvalidMessageStatus, status)
	}
	return nil
}
