package orchestrator

import (
	"fmt"
	"testing"
	"time"
)

func TestClassifyFailure_Retryable(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		output string
	}{
		{"timeout error", fmt.Errorf("context deadline exceeded"), ""},
		{"connection refused", fmt.Errorf("connection refused"), ""},
		{"rate limit", nil, "429 Too Many Requests"},
		{"network error", fmt.Errorf("network unreachable"), ""},
		{"timed out", fmt.Errorf("operation timed out"), ""},
		{"503 in output", nil, "HTTP 503 Service Unavailable"},
		{"temporary failure", fmt.Errorf("temporary failure"), ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			class := ClassifyFailure(tt.err, tt.output)
			if class != FailureRetryable {
				t.Errorf("expected FailureRetryable, got %s", class)
			}
		})
	}
}

func TestClassifyFailure_Terminal(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		output string
	}{
		{"compile error", nil, "compile error: undefined variable"},
		{"permission denied", fmt.Errorf("permission denied"), ""},
		{"syntax error", nil, "syntax error at line 42"},
		{"unauthorized", fmt.Errorf("unauthorized access"), ""},
		{"missing required", nil, "missing required field: name"},
		{"type error", nil, "type error: cannot assign string to int"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			class := ClassifyFailure(tt.err, tt.output)
			if class != FailureTerminal {
				t.Errorf("expected FailureTerminal, got %s", class)
			}
		})
	}
}

func TestClassifyFailure_Unknown(t *testing.T) {
	class := ClassifyFailure(fmt.Errorf("something weird happened"), "no recognizable pattern")
	if class != FailureUnknown {
		t.Errorf("expected FailureUnknown, got %s", class)
	}
}

func TestClassifyFailure_NilError(t *testing.T) {
	class := ClassifyFailure(nil, "")
	if class != FailureUnknown {
		t.Errorf("expected FailureUnknown for nil error and empty output, got %s", class)
	}
}

func TestRetryPolicy_NextDelay(t *testing.T) {
	policy := DefaultRetryPolicy()

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{0, 5 * time.Second},                // 5s * 2^0 = 5s
		{1, 10 * time.Second},               // 5s * 2^1 = 10s
		{2, 20 * time.Second},               // 5s * 2^2 = 20s
		{3, 40 * time.Second},               // 5s * 2^3 = 40s
		{4, 80 * time.Second},               // 5s * 2^4 = 80s
		{5, 2 * time.Minute},                // 5s * 2^5 = 160s, capped at 2m
		{10, 2 * time.Minute},               // capped at MaxDelay
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("attempt_%d", tt.attempt), func(t *testing.T) {
			delay := policy.NextDelay(tt.attempt)
			if delay != tt.expected {
				t.Errorf("attempt %d: expected %v, got %v", tt.attempt, tt.expected, delay)
			}
		})
	}
}

func TestRetryPolicy_NextDelay_NegativeAttempt(t *testing.T) {
	policy := DefaultRetryPolicy()
	delay := policy.NextDelay(-1)
	if delay != 5*time.Second {
		t.Errorf("negative attempt should return InitialDelay, got %v", delay)
	}
}

func TestDefaultRetryPolicy(t *testing.T) {
	policy := DefaultRetryPolicy()
	if policy.MaxAttempts != 3 {
		t.Errorf("MaxAttempts: expected 3, got %d", policy.MaxAttempts)
	}
	if policy.InitialDelay != 5*time.Second {
		t.Errorf("InitialDelay: expected 5s, got %v", policy.InitialDelay)
	}
	if policy.MaxDelay != 2*time.Minute {
		t.Errorf("MaxDelay: expected 2m, got %v", policy.MaxDelay)
	}
	if policy.Multiplier != 2.0 {
		t.Errorf("Multiplier: expected 2.0, got %f", policy.Multiplier)
	}
}
