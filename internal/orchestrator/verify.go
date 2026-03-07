package orchestrator

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// VerifyCommand defines a single verification step from verify.yml.
type VerifyCommand struct {
	Name    string `yaml:"name"`
	Command string `yaml:"command"`
	Timeout string `yaml:"timeout"`
}

// VerifyResult holds the outcome of a single verification step.
type VerifyResult struct {
	Name    string
	Command string
	Success bool
	Output  string
	Elapsed time.Duration
}

// RunVerification executes all verification commands and returns results.
// Spec Reference: Section 7 "pylon request" step 9 (cross-validation)
func RunVerification(projectDir string, commands []VerifyCommand) ([]VerifyResult, error) {
	var results []VerifyResult

	for _, vc := range commands {
		timeout, err := time.ParseDuration(vc.Timeout)
		if err != nil {
			timeout = 5 * time.Minute
		}

		result := runSingleVerification(projectDir, vc, timeout)
		results = append(results, result)
	}

	return results, nil
}

func runSingleVerification(dir string, vc VerifyCommand, timeout time.Duration) VerifyResult {
	start := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	parts := strings.Fields(vc.Command)
	if len(parts) == 0 {
		return VerifyResult{
			Name:    vc.Name,
			Command: vc.Command,
			Success: false,
			Output:  "empty command",
			Elapsed: 0,
		}
	}

	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	cmd.Dir = dir

	output, err := cmd.CombinedOutput()
	elapsed := time.Since(start)

	return VerifyResult{
		Name:    vc.Name,
		Command: vc.Command,
		Success: err == nil,
		Output:  string(output),
		Elapsed: elapsed,
	}
}

// AllPassed returns true if all verification results succeeded.
func AllPassed(results []VerifyResult) bool {
	for _, r := range results {
		if !r.Success {
			return false
		}
	}
	return true
}

// FailedSummary returns a summary of failed verifications.
func FailedSummary(results []VerifyResult) string {
	var failed []string
	for _, r := range results {
		if !r.Success {
			failed = append(failed, fmt.Sprintf("  ✗ %s (%s): %s", r.Name, r.Elapsed, truncateOutput(r.Output, 200)))
		}
	}
	return strings.Join(failed, "\n")
}

func truncateOutput(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}
