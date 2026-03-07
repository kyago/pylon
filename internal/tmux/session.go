// Package tmux provides tmux session management for pylon agent processes.
// Spec Reference: Section 8 "Process Management: tmux Sessions"
package tmux

import "time"

// SessionManager defines the interface for tmux session operations.
// Abstracted as an interface for testability via mocking.
type SessionManager interface {
	// Create starts a new tmux session with the given configuration.
	Create(cfg SessionConfig) error

	// Kill terminates a tmux session by name.
	Kill(name string) error

	// IsAlive checks whether a tmux session with the given name exists.
	IsAlive(name string) bool

	// List returns all tmux sessions matching the configured prefix.
	List() ([]SessionInfo, error)

	// SendKeys sends keystrokes to a tmux session.
	SendKeys(name string, keys string) error

	// CapturePane captures the last N lines of output from a tmux pane.
	CapturePane(name string, lines int) (string, error)
}

// SessionConfig holds parameters for creating a new tmux session.
type SessionConfig struct {
	// Name is the session name, typically "{prefix}-{agent-name}".
	Name string

	// WorkDir is the working directory for the session.
	WorkDir string

	// Command is the shell command to execute inside the session.
	Command string

	// Env contains environment variables to set in the session.
	Env map[string]string

	// HistoryLimit controls the scrollback buffer size (tmux history-limit).
	HistoryLimit int
}

// SessionInfo holds metadata about an existing tmux session.
type SessionInfo struct {
	// Name is the tmux session name.
	Name string

	// Created is the time when the session was created.
	Created time.Time

	// Activity is the time of last activity in the session.
	Activity time.Time

	// Alive indicates whether the session is currently running.
	Alive bool
}
