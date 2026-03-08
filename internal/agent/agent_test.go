package agent

import (
	"strings"
	"testing"
	"time"

	"github.com/kyago/pylon/internal/config"
)

// --- Env Tests ---

func TestResolveEnv_Merge(t *testing.T) {
	global := map[string]string{
		"CLAUDE_CODE_EFFORT_LEVEL":        "high",
		"CLAUDE_AUTOCOMPACT_PCT_OVERRIDE": "80",
	}
	agent := map[string]string{
		"CLAUDE_CODE_EFFORT_LEVEL": "medium",
		"CUSTOM_VAR":               "value",
	}

	result := ResolveEnv(global, agent)

	if result["CLAUDE_CODE_EFFORT_LEVEL"] != "medium" {
		t.Error("agent env should override global")
	}
	if result["CLAUDE_AUTOCOMPACT_PCT_OVERRIDE"] != "80" {
		t.Error("global env should be preserved")
	}
	if result["CUSTOM_VAR"] != "value" {
		t.Error("agent-only env should be included")
	}
}

func TestResolveEnv_NilMaps(t *testing.T) {
	result := ResolveEnv(nil, nil)
	if len(result) != 0 {
		t.Error("expected empty map for nil inputs")
	}
}

// --- Lifecycle Tests ---

func TestLifecycle_ValidTransitions(t *testing.T) {
	l := NewLifecycle("dev", "t1", "pylon-dev", 30*time.Minute)

	if l.State != StateIdle {
		t.Errorf("expected idle, got %q", l.State)
	}

	transitions := []State{StateStarting, StateRunning, StateCompleted}
	for _, s := range transitions {
		if err := l.Transition(s); err != nil {
			t.Errorf("transition to %q failed: %v", s, err)
		}
	}

	if !l.IsTerminal() {
		t.Error("completed should be terminal")
	}
}

func TestLifecycle_InvalidTransition(t *testing.T) {
	l := NewLifecycle("dev", "t1", "pylon-dev", 0)

	// Can't go from idle to running directly
	err := l.Transition(StateRunning)
	if err == nil {
		t.Error("expected error for idle → running")
	}
}

func TestLifecycle_FailFromStarting(t *testing.T) {
	l := NewLifecycle("dev", "t1", "pylon-dev", 0)
	l.Transition(StateStarting)

	if err := l.Transition(StateFailed); err != nil {
		t.Errorf("starting → failed should be valid: %v", err)
	}
}

func TestLifecycle_CancelFromRunning(t *testing.T) {
	l := NewLifecycle("dev", "t1", "pylon-dev", 0)
	l.Transition(StateStarting)
	l.Transition(StateRunning)

	if err := l.Transition(StateCancelled); err != nil {
		t.Errorf("running → cancelled should be valid: %v", err)
	}
}

func TestLifecycle_NoTransitionFromTerminal(t *testing.T) {
	l := NewLifecycle("dev", "t1", "pylon-dev", 0)
	l.Transition(StateStarting)
	l.Transition(StateRunning)
	l.Transition(StateCompleted)

	err := l.Transition(StateRunning)
	if err == nil {
		t.Error("should not transition from terminal state")
	}
}

func TestLifecycle_CheckTimeout(t *testing.T) {
	l := NewLifecycle("dev", "t1", "pylon-dev", 1*time.Millisecond)
	l.Transition(StateStarting)
	l.Transition(StateRunning)

	time.Sleep(2 * time.Millisecond)
	if !l.CheckTimeout() {
		t.Error("expected timeout")
	}
}

func TestLifecycle_NoTimeout(t *testing.T) {
	l := NewLifecycle("dev", "t1", "pylon-dev", 10*time.Minute)
	l.Transition(StateStarting)
	l.Transition(StateRunning)

	if l.CheckTimeout() {
		t.Error("should not be timed out")
	}
}

func TestLifecycle_TimeoutDisabled(t *testing.T) {
	l := NewLifecycle("dev", "t1", "pylon-dev", 0)
	l.Transition(StateStarting)
	l.Transition(StateRunning)

	if l.CheckTimeout() {
		t.Error("timeout 0 means disabled")
	}
}

// --- CLAUDE.md Builder Tests ---

func TestClaudeMDBuilder_Build(t *testing.T) {
	builder := &ClaudeMDBuilder{MaxLines: 200}

	result, err := builder.Build(BuildInput{
		CommunicationRules: DefaultCommunicationRules(),
		TaskContext:        "JWT 기반 로그인 구현\n수용 기준: POST /auth/login",
		CompactionRules:    DefaultCompactionRules(),
		ProjectMemory:      "이전 패턴: Echo 미들웨어 기반",
		DomainPaths:        []string{".pylon/domain/architecture.md", ".pylon/domain/conventions.md"},
	})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	lines := strings.Split(result, "\n")
	if len(lines) > 200 {
		t.Errorf("exceeded 200-line limit: %d lines", len(lines))
	}

	// Check sections are present
	if !strings.Contains(result, "Communication Rules") {
		t.Error("missing Communication Rules section")
	}
	if !strings.Contains(result, "Task Context") {
		t.Error("missing Task Context section")
	}
	if !strings.Contains(result, "Domain Knowledge") {
		t.Error("missing Domain Knowledge section")
	}
}

func TestClaudeMDBuilder_TruncatesLowPriority(t *testing.T) {
	builder := &ClaudeMDBuilder{MaxLines: 10}

	result, err := builder.Build(BuildInput{
		CommunicationRules: "line1\nline2\nline3\nline4\nline5",
		TaskContext:        "line1\nline2\nline3\nline4\nline5",
		ProjectMemory:      "this should be truncated",
		DomainPaths:        []string{"path1", "path2"},
	})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	lines := strings.Split(result, "\n")
	if len(lines) > 10 {
		t.Errorf("exceeded 10-line limit: %d lines", len(lines))
	}

	// Domain Knowledge should not appear (too low priority)
	if strings.Contains(result, "Domain Knowledge") {
		t.Error("low-priority section should be truncated")
	}
}

func TestClaudeMDBuilder_EmptyInput(t *testing.T) {
	builder := &ClaudeMDBuilder{MaxLines: 200}

	result, err := builder.Build(BuildInput{})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if result != "" {
		t.Errorf("expected empty result for empty input, got %q", result)
	}
}

func TestClaudeMDBuilder_DefaultMaxLines(t *testing.T) {
	builder := &ClaudeMDBuilder{} // MaxLines=0 → default

	_, err := builder.Build(BuildInput{
		CommunicationRules: "test",
	})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
}

// --- Runner Tests ---

func TestRunner_BuildCommand_NonInteractive(t *testing.T) {
	r := NewRunner(nil)

	cmd := r.BuildCommand(RunConfig{
		Agent: &config.AgentConfig{
			Name:           "backend-dev",
			MaxTurns:       30,
			PermissionMode: "acceptEdits",
			Model:          "sonnet",
		},
		Global:     &config.Config{},
		TaskPrompt: "Implement login API",
		ClaudeMD:   "test rules",
	})

	if !strings.Contains(cmd, "--print") {
		t.Error("non-interactive should have --print")
	}
	if !strings.Contains(cmd, "--output-format stream-json") {
		t.Error("should have stream-json format")
	}
	if !strings.Contains(cmd, "--max-turns 30") {
		t.Error("should have max-turns")
	}
	if !strings.Contains(cmd, "--permission-mode acceptEdits") {
		t.Error("should have permission-mode")
	}
	if !strings.Contains(cmd, "--model sonnet") {
		t.Error("should have model")
	}
	if !strings.Contains(cmd, "--prompt") {
		t.Error("non-interactive should have --prompt")
	}
}

func TestRunner_BuildCommand_Interactive(t *testing.T) {
	r := NewRunner(nil)

	cmd := r.BuildCommand(RunConfig{
		Agent: &config.AgentConfig{
			Name:           "po",
			MaxTurns:       50,
			PermissionMode: "default",
		},
		Global:      &config.Config{},
		Interactive: true,
	})

	if strings.Contains(cmd, "--print") {
		t.Error("interactive should NOT have --print")
	}
	if strings.Contains(cmd, "--prompt") {
		t.Error("interactive should NOT have --prompt")
	}
	if !strings.Contains(cmd, "--permission-mode default") {
		t.Error("should have permission-mode")
	}
}

func TestRunner_BuildCommand_NoModel(t *testing.T) {
	r := NewRunner(nil)

	cmd := r.BuildCommand(RunConfig{
		Agent: &config.AgentConfig{
			Name:           "dev",
			PermissionMode: "acceptEdits",
		},
		Global: &config.Config{},
	})

	if strings.Contains(cmd, "--model") {
		t.Error("should not have --model when not specified")
	}
}
