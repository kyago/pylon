package dashboard

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// NewFileLogger creates a logger that writes to dashboard.log in the given directory.
// The caller must close the returned file when done.
func NewFileLogger(logDir string) (*log.Logger, *os.File, error) {
	if err := os.MkdirAll(logDir, 0700); err != nil {
		return nil, nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	logPath := filepath.Join(logDir, "dashboard.log")
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open log file: %w", err)
	}

	logger := log.New(f, "dashboard: ", log.LstdFlags)
	return logger, f, nil
}
