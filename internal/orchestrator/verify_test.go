package orchestrator

import (
	"testing"
)

func TestRunVerification_Success(t *testing.T) {
	results, err := RunVerification(t.TempDir(), []VerifyCommand{
		{Name: "echo test", Command: "echo hello", Timeout: "10s"},
	})
	if err != nil {
		t.Fatalf("RunVerification failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Success {
		t.Errorf("expected success, got failure: %s", results[0].Output)
	}
	if results[0].Name != "echo test" {
		t.Errorf("name = %s, want echo test", results[0].Name)
	}
}

func TestRunVerification_Failure(t *testing.T) {
	results, err := RunVerification(t.TempDir(), []VerifyCommand{
		{Name: "false cmd", Command: "false", Timeout: "10s"},
	})
	if err != nil {
		t.Fatalf("RunVerification failed: %v", err)
	}
	if results[0].Success {
		t.Error("expected failure for 'false' command")
	}
}

func TestRunVerification_InvalidTimeout(t *testing.T) {
	results, err := RunVerification(t.TempDir(), []VerifyCommand{
		{Name: "test", Command: "echo ok", Timeout: "invalid"},
	})
	if err != nil {
		t.Fatalf("RunVerification failed: %v", err)
	}
	// Should use default 5m timeout and still succeed
	if !results[0].Success {
		t.Error("expected success with default timeout")
	}
}

func TestRunVerification_EmptyCommand(t *testing.T) {
	results, err := RunVerification(t.TempDir(), []VerifyCommand{
		{Name: "empty", Command: "", Timeout: "10s"},
	})
	if err != nil {
		t.Fatalf("RunVerification failed: %v", err)
	}
	if results[0].Success {
		t.Error("expected failure for empty command")
	}
	if results[0].Output != "empty command" {
		t.Errorf("output = %s, want 'empty command'", results[0].Output)
	}
}

func TestRunVerification_Multiple(t *testing.T) {
	results, err := RunVerification(t.TempDir(), []VerifyCommand{
		{Name: "pass", Command: "echo pass", Timeout: "10s"},
		{Name: "fail", Command: "false", Timeout: "10s"},
		{Name: "pass2", Command: "echo pass2", Timeout: "10s"},
	})
	if err != nil {
		t.Fatalf("RunVerification failed: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if !results[0].Success || results[1].Success || !results[2].Success {
		t.Error("unexpected success/failure pattern")
	}
}

func TestAllPassed(t *testing.T) {
	tests := []struct {
		name     string
		results  []VerifyResult
		expected bool
	}{
		{"all pass", []VerifyResult{{Success: true}, {Success: true}}, true},
		{"one fail", []VerifyResult{{Success: true}, {Success: false}}, false},
		{"empty", []VerifyResult{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AllPassed(tt.results); got != tt.expected {
				t.Errorf("AllPassed = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestFailedSummary(t *testing.T) {
	results := []VerifyResult{
		{Name: "build", Success: true},
		{Name: "test", Success: false, Output: "FAIL: test_foo", Elapsed: 100},
		{Name: "lint", Success: false, Output: "error: unused var", Elapsed: 50},
	}
	summary := FailedSummary(results)
	if summary == "" {
		t.Error("summary should not be empty when there are failures")
	}
	if !contains(summary, "test") || !contains(summary, "lint") {
		t.Errorf("summary should contain failed test names: %s", summary)
	}
	if contains(summary, "build") {
		t.Error("summary should not contain passed tests")
	}
}

func TestFailedSummary_NoFailures(t *testing.T) {
	results := []VerifyResult{{Name: "ok", Success: true}}
	if summary := FailedSummary(results); summary != "" {
		t.Errorf("expected empty summary, got: %s", summary)
	}
}

func TestTruncateOutput(t *testing.T) {
	short := "hello"
	if got := truncateOutput(short, 10); got != "hello" {
		t.Errorf("got %s, want hello", got)
	}

	long := "abcdefghijklmnop"
	if got := truncateOutput(long, 5); got != "abcde..." {
		t.Errorf("got %s, want abcde...", got)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
