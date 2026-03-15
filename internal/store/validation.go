package store

import (
	"errors"
	"fmt"
	"math"
)

// Validation errors returned when input values violate constraints.
var (
	ErrConfidenceOutOfRange = errors.New("confidence must be between 0.0 and 1.0")
	ErrInvalidPipelineStage = errors.New("invalid pipeline stage")
	ErrInvalidMessageStatus = errors.New("invalid message queue status")
)

// NOTE: 이 목록은 반드시 internal/orchestrator/pipeline.go의 Stage 상수와 동기화해야 합니다.
var validPipelineStages = map[string]bool{
	"init":               true,
	"po_conversation":    true,
	"architect_analysis": true,
	"pm_task_breakdown":  true,
	"agent_executing":    true,
	"verification":       true,
	"pr_creation":        true,
	"po_validation":      true,
	"wiki_update":        true,
	"completed":          true,
	"failed":             true,
}

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
