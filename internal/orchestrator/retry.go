package orchestrator

import (
	"math"
	"strings"
	"time"
)

// FailureClass categorizes a failure for retry decisions.
type FailureClass int

const (
	// FailureRetryable indicates the failure is transient and can be retried.
	FailureRetryable FailureClass = iota
	// FailureTerminal indicates the failure is permanent and should not be retried.
	FailureTerminal
	// FailureUnknown indicates the failure could not be classified.
	FailureUnknown
)

// String returns the string representation of a FailureClass.
func (fc FailureClass) String() string {
	switch fc {
	case FailureRetryable:
		return "retryable"
	case FailureTerminal:
		return "terminal"
	default:
		return "unknown"
	}
}

// retryablePatterns are error message substrings that indicate transient failures.
var retryablePatterns = []string{
	"timeout",
	"timed out",
	"deadline exceeded",
	"connection refused",
	"connection reset",
	"network",
	"rate limit",
	"too many requests",
	"429",
	"503",
	"502",
	"temporary",
	"EAGAIN",
	"ECONNRESET",
	"ETIMEDOUT",
}

// terminalPatterns are error message substrings that indicate permanent failures.
var terminalPatterns = []string{
	"compile error",
	"compilation failed",
	"syntax error",
	"permission denied",
	"access denied",
	"unauthorized",
	"forbidden",
	"not found",
	"invalid config",
	"invalid configuration",
	"missing required",
	"type error",
	"undefined reference",
}

// ClassifyFailure categorizes a failure based on error message and output content.
// It checks for known retryable patterns first, then terminal patterns.
// If neither matches, it returns FailureUnknown (treated as retryable by default).
func ClassifyFailure(err error, output string) FailureClass {
	combined := ""
	if err != nil {
		combined = strings.ToLower(err.Error())
	}
	if output != "" {
		combined += " " + strings.ToLower(output)
	}

	for _, pattern := range retryablePatterns {
		if strings.Contains(combined, pattern) {
			return FailureRetryable
		}
	}

	for _, pattern := range terminalPatterns {
		if strings.Contains(combined, pattern) {
			return FailureTerminal
		}
	}

	return FailureUnknown
}

// RetryPolicy defines the retry behavior with exponential backoff.
type RetryPolicy struct {
	MaxAttempts  int
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
}

// DefaultRetryPolicy returns a sensible default retry policy.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts:  3,
		InitialDelay: 5 * time.Second,
		MaxDelay:     2 * time.Minute,
		Multiplier:   2.0,
	}
}

// NextDelay calculates the delay before the next retry attempt using exponential backoff.
// attempt is 0-indexed (0 = first retry).
func (rp RetryPolicy) NextDelay(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	delay := float64(rp.InitialDelay) * math.Pow(rp.Multiplier, float64(attempt))
	if delay > float64(rp.MaxDelay) {
		delay = float64(rp.MaxDelay)
	}
	return time.Duration(delay)
}
