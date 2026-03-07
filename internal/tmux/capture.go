package tmux

import (
	"fmt"
	"strings"
	"time"
)

// CaptureResult holds the result of a pane capture operation.
type CaptureResult struct {
	// SessionName is the tmux session that was captured.
	SessionName string

	// Output is the captured text content.
	Output string

	// CapturedAt is the time the capture was taken.
	CapturedAt time.Time

	// LineCount is the number of non-empty lines captured.
	LineCount int
}

// CaptureSession captures a session's pane output and returns structured result.
func CaptureSession(mgr SessionManager, sessionName string, lines int) (*CaptureResult, error) {
	if !mgr.IsAlive(sessionName) {
		return nil, fmt.Errorf("session %q is not alive", sessionName)
	}

	output, err := mgr.CapturePane(sessionName, lines)
	if err != nil {
		return nil, err
	}

	// Count non-empty lines
	lineCount := 0
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) != "" {
			lineCount++
		}
	}

	return &CaptureResult{
		SessionName: sessionName,
		Output:      output,
		CapturedAt:  time.Now(),
		LineCount:   lineCount,
	}, nil
}

// CaptureAll captures output from all sessions managed by the given manager.
func CaptureAll(mgr SessionManager, lines int) ([]CaptureResult, error) {
	sessions, err := mgr.List()
	if err != nil {
		return nil, err
	}

	var results []CaptureResult
	for _, s := range sessions {
		result, err := CaptureSession(mgr, s.Name, lines)
		if err != nil {
			// Log but continue capturing other sessions
			results = append(results, CaptureResult{
				SessionName: s.Name,
				Output:      fmt.Sprintf("capture error: %v", err),
				CapturedAt:  time.Now(),
			})
			continue
		}
		results = append(results, *result)
	}

	return results, nil
}
