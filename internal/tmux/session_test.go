package tmux

import (
	"fmt"
	"testing"
	"time"
)

// mockManager implements SessionManager for unit testing.
type mockManager struct {
	sessions map[string]SessionInfo
	captures map[string]string
	sentKeys map[string][]string
}

func newMockManager() *mockManager {
	return &mockManager{
		sessions: make(map[string]SessionInfo),
		captures: make(map[string]string),
		sentKeys: make(map[string][]string),
	}
}

func (m *mockManager) Create(cfg SessionConfig) error {
	if cfg.Name == "" {
		return fmt.Errorf("session name is required")
	}
	if _, exists := m.sessions[cfg.Name]; exists {
		return fmt.Errorf("tmux session %q already exists", cfg.Name)
	}
	m.sessions[cfg.Name] = SessionInfo{
		Name:     cfg.Name,
		Created:  time.Now(),
		Activity: time.Now(),
		Alive:    true,
	}
	return nil
}

func (m *mockManager) Kill(name string) error {
	if _, exists := m.sessions[name]; !exists {
		return fmt.Errorf("session %q not found", name)
	}
	delete(m.sessions, name)
	return nil
}

func (m *mockManager) IsAlive(name string) bool {
	_, exists := m.sessions[name]
	return exists
}

func (m *mockManager) List() ([]SessionInfo, error) {
	var result []SessionInfo
	for _, s := range m.sessions {
		result = append(result, s)
	}
	return result, nil
}

func (m *mockManager) SendKeys(name string, keys string) error {
	if _, exists := m.sessions[name]; !exists {
		return fmt.Errorf("session %q not found", name)
	}
	m.sentKeys[name] = append(m.sentKeys[name], keys)
	return nil
}

func (m *mockManager) CapturePane(name string, lines int) (string, error) {
	if _, exists := m.sessions[name]; !exists {
		return "", fmt.Errorf("session %q not found", name)
	}
	output, ok := m.captures[name]
	if !ok {
		return "", nil
	}
	return output, nil
}

func TestMockManager_Create(t *testing.T) {
	mgr := newMockManager()

	err := mgr.Create(SessionConfig{Name: "pylon-po", WorkDir: "/tmp"})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if !mgr.IsAlive("pylon-po") {
		t.Error("session should be alive after creation")
	}
}

func TestMockManager_CreateDuplicate(t *testing.T) {
	mgr := newMockManager()
	mgr.Create(SessionConfig{Name: "pylon-po"})

	err := mgr.Create(SessionConfig{Name: "pylon-po"})
	if err == nil {
		t.Error("expected error for duplicate session")
	}
}

func TestMockManager_CreateEmptyName(t *testing.T) {
	mgr := newMockManager()

	err := mgr.Create(SessionConfig{Name: ""})
	if err == nil {
		t.Error("expected error for empty name")
	}
}

func TestMockManager_Kill(t *testing.T) {
	mgr := newMockManager()
	mgr.Create(SessionConfig{Name: "pylon-po"})

	err := mgr.Kill("pylon-po")
	if err != nil {
		t.Fatalf("Kill failed: %v", err)
	}

	if mgr.IsAlive("pylon-po") {
		t.Error("session should not be alive after kill")
	}
}

func TestMockManager_KillNotFound(t *testing.T) {
	mgr := newMockManager()

	err := mgr.Kill("nonexistent")
	if err == nil {
		t.Error("expected error for killing nonexistent session")
	}
}

func TestMockManager_List(t *testing.T) {
	mgr := newMockManager()
	mgr.Create(SessionConfig{Name: "pylon-po"})
	mgr.Create(SessionConfig{Name: "pylon-pm"})

	sessions, err := mgr.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestMockManager_SendKeys(t *testing.T) {
	mgr := newMockManager()
	mgr.Create(SessionConfig{Name: "pylon-po"})

	err := mgr.SendKeys("pylon-po", "echo hello Enter")
	if err != nil {
		t.Fatalf("SendKeys failed: %v", err)
	}

	if len(mgr.sentKeys["pylon-po"]) != 1 {
		t.Error("expected 1 sent key entry")
	}
}

func TestMockManager_CapturePane(t *testing.T) {
	mgr := newMockManager()
	mgr.Create(SessionConfig{Name: "pylon-po"})
	mgr.captures["pylon-po"] = "line1\nline2\nline3"

	output, err := mgr.CapturePane("pylon-po", 100)
	if err != nil {
		t.Fatalf("CapturePane failed: %v", err)
	}

	if output != "line1\nline2\nline3" {
		t.Errorf("unexpected output: %s", output)
	}
}

func TestParseSessionLine(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		prefix   string
		wantName string
		wantNil  bool
		wantErr  bool
	}{
		{
			name:     "valid pylon session",
			line:     "pylon-po:1709827200:1709827260",
			prefix:   "pylon",
			wantName: "pylon-po",
		},
		{
			name:    "non-pylon session skipped",
			line:    "other-session:1709827200:1709827260",
			prefix:  "pylon",
			wantNil: true,
		},
		{
			name:    "invalid format",
			line:    "badformat",
			prefix:  "pylon",
			wantErr: true,
		},
		{
			name:    "invalid created timestamp",
			line:    "pylon-po:notanumber:1709827260",
			prefix:  "pylon",
			wantErr: true,
		},
		{
			name:    "invalid activity timestamp",
			line:    "pylon-po:1709827200:notanumber",
			prefix:  "pylon",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := parseSessionLine(tt.line, tt.prefix)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantNil {
				if info != nil {
					t.Error("expected nil result for non-matching prefix")
				}
				return
			}
			if info.Name != tt.wantName {
				t.Errorf("expected name %q, got %q", tt.wantName, info.Name)
			}
			if !info.Alive {
				t.Error("parsed session should be alive")
			}
		})
	}
}

func TestCaptureSession(t *testing.T) {
	mgr := newMockManager()
	mgr.Create(SessionConfig{Name: "pylon-po"})
	mgr.captures["pylon-po"] = "output line 1\noutput line 2\n\n"

	result, err := CaptureSession(mgr, "pylon-po", 100)
	if err != nil {
		t.Fatalf("CaptureSession failed: %v", err)
	}

	if result.SessionName != "pylon-po" {
		t.Errorf("expected session name pylon-po, got %s", result.SessionName)
	}
	if result.LineCount != 2 {
		t.Errorf("expected 2 non-empty lines, got %d", result.LineCount)
	}
}

func TestCaptureSession_DeadSession(t *testing.T) {
	mgr := newMockManager()

	_, err := CaptureSession(mgr, "pylon-dead", 100)
	if err == nil {
		t.Error("expected error for dead session")
	}
}

func TestCaptureAll(t *testing.T) {
	mgr := newMockManager()
	mgr.Create(SessionConfig{Name: "pylon-po"})
	mgr.Create(SessionConfig{Name: "pylon-pm"})
	mgr.captures["pylon-po"] = "output1"
	mgr.captures["pylon-pm"] = "output2"

	results, err := CaptureAll(mgr, 100)
	if err != nil {
		t.Fatalf("CaptureAll failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestNewManager(t *testing.T) {
	m := NewManager("")
	if m.Prefix != "pylon" {
		t.Errorf("expected default prefix 'pylon', got %q", m.Prefix)
	}

	m2 := NewManager("custom")
	if m2.Prefix != "custom" {
		t.Errorf("expected prefix 'custom', got %q", m2.Prefix)
	}
}

func TestSessionName(t *testing.T) {
	m := NewManager("pylon")
	name := m.SessionName("po")
	if name != "pylon-po" {
		t.Errorf("expected 'pylon-po', got %q", name)
	}
}
