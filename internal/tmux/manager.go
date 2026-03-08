package tmux

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Manager implements SessionManager using real tmux commands.
type Manager struct {
	// Prefix is the session name prefix for filtering (default: "pylon").
	Prefix string
}

// NewManager creates a new tmux Manager with the given session prefix.
func NewManager(prefix string) *Manager {
	if prefix == "" {
		prefix = "pylon"
	}
	return &Manager{Prefix: prefix}
}

// SessionName returns the full tmux session name for an agent.
func (m *Manager) SessionName(agentName string) string {
	return m.Prefix + "-" + agentName
}

// Create starts a new detached tmux session.
// It sets the working directory and environment, then sends the command.
func (m *Manager) Create(cfg SessionConfig) error {
	if cfg.Name == "" {
		return fmt.Errorf("session name is required")
	}

	// Check if session already exists
	if m.IsAlive(cfg.Name) {
		return fmt.Errorf("tmux session %q already exists", cfg.Name)
	}

	// Build tmux new-session command
	args := []string{
		"new-session",
		"-d",
		"-s", cfg.Name,
	}
	if cfg.WorkDir != "" {
		args = append(args, "-c", cfg.WorkDir)
	}

	// Set history limit via -x environment
	if cfg.HistoryLimit > 0 {
		args = append(args, "-x", "200", "-y", "50")
	}

	cmd := exec.Command("tmux", args...)

	// Set environment variables
	if len(cfg.Env) > 0 {
		cmd.Env = cmd.Environ()
		for k, v := range cfg.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create tmux session %q: %w\n%s", cfg.Name, err, output)
	}

	// Set history-limit if specified
	if cfg.HistoryLimit > 0 {
		setOpt := exec.Command("tmux", "set-option", "-t", cfg.Name, "history-limit", strconv.Itoa(cfg.HistoryLimit))
		setOpt.CombinedOutput() // best-effort
	}

	// Send the command to execute
	if cfg.Command != "" {
		// Use -l flag for literal text (prevents tmux key name interpretation
		// of sequences like "Enter", "C-c" that may appear in command content)
		sendCmd := exec.Command("tmux", "send-keys", "-t", cfg.Name, "-l", cfg.Command)
		if output, err := sendCmd.CombinedOutput(); err != nil {
			m.Kill(cfg.Name)
			return fmt.Errorf("failed to send command to session %q: %w\n%s", cfg.Name, err, output)
		}
		// Send Enter key separately to execute the command
		enterCmd := exec.Command("tmux", "send-keys", "-t", cfg.Name, "Enter")
		if output, err := enterCmd.CombinedOutput(); err != nil {
			m.Kill(cfg.Name)
			return fmt.Errorf("failed to send Enter to session %q: %w\n%s", cfg.Name, err, output)
		}
	}

	return nil
}

// Kill terminates a tmux session by name.
func (m *Manager) Kill(name string) error {
	cmd := exec.Command("tmux", "kill-session", "-t", name)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to kill tmux session %q: %w\n%s", name, err, output)
	}
	return nil
}

// IsAlive checks whether a tmux session with the given name exists.
func (m *Manager) IsAlive(name string) bool {
	cmd := exec.Command("tmux", "has-session", "-t", name)
	return cmd.Run() == nil
}

// List returns all tmux sessions whose names start with the configured prefix.
func (m *Manager) List() ([]SessionInfo, error) {
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}:#{session_created}:#{session_activity}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// "no server running" or "no sessions" is not an error for us
		outStr := string(output)
		if strings.Contains(outStr, "no server running") || strings.Contains(outStr, "no sessions") || strings.Contains(outStr, "error connecting to") {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list tmux sessions: %w\n%s", err, output)
	}

	var sessions []SessionInfo
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		info, err := parseSessionLine(line, m.Prefix)
		if err != nil {
			continue // skip unparseable lines
		}
		if info != nil {
			sessions = append(sessions, *info)
		}
	}

	return sessions, nil
}

// parseSessionLine parses a tmux list-sessions format line.
// Format: "session_name:created_epoch:activity_epoch"
// Returns nil if the session doesn't match the prefix filter.
func parseSessionLine(line string, prefix string) (*SessionInfo, error) {
	parts := strings.SplitN(line, ":", 3)
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid session line: %s", line)
	}

	name := parts[0]
	if !strings.HasPrefix(name, prefix+"-") {
		return nil, nil // not a pylon session, skip
	}

	created, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid created timestamp: %s", parts[1])
	}

	activity, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid activity timestamp: %s", parts[2])
	}

	return &SessionInfo{
		Name:     name,
		Created:  time.Unix(created, 0),
		Activity: time.Unix(activity, 0),
		Alive:    true,
	}, nil
}

// SendKeys sends keystrokes to a tmux session.
func (m *Manager) SendKeys(name string, keys string) error {
	cmd := exec.Command("tmux", "send-keys", "-t", name, keys)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to send keys to session %q: %w\n%s", name, err, output)
	}
	return nil
}

// CapturePane captures the last N lines from a tmux session's pane.
func (m *Manager) CapturePane(name string, lines int) (string, error) {
	if lines <= 0 {
		lines = 100
	}
	cmd := exec.Command("tmux", "capture-pane", "-t", name, "-p", "-S", fmt.Sprintf("-%d", lines))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to capture pane for session %q: %w\n%s", name, err, output)
	}
	return string(output), nil
}

// KillAllWithPrefix terminates all tmux sessions matching the prefix.
// Returns the number of sessions killed and any errors encountered.
func (m *Manager) KillAllWithPrefix() (int, error) {
	sessions, err := m.List()
	if err != nil {
		return 0, err
	}

	killed := 0
	var errs []string
	for _, s := range sessions {
		if err := m.Kill(s.Name); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", s.Name, err))
		} else {
			killed++
		}
	}

	if len(errs) > 0 {
		return killed, fmt.Errorf("failed to kill some sessions:\n%s", strings.Join(errs, "\n"))
	}
	return killed, nil
}
