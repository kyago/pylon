package store

import (
	"errors"
	"fmt"
	"math"
)

// Validation errors returned when input values violate constraints.
var ErrConfidenceOutOfRange = errors.New("confidence must be between 0.0 and 1.0")

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
