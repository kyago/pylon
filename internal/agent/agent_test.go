package agent

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/executor"
	"github.com/kyago/pylon/internal/git"
)

// mockExecutor records calls to RunHeadless for testing.
type mockExecutor struct {
	lastCfg executor.ExecConfig
	result  *executor.ExecResult
	err     error
}

func (m *mockExecutor) ExecInteractive(cfg executor.ExecConfig) error {
	return fmt.Errorf("not implemented in mock")
}

func (m *mockExecutor) RunHeadless(cfg executor.ExecConfig) (*executor.ExecResult, error) {
	m.lastCfg = cfg
	return m.result, m.err
}

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
	l := NewLifecycle("dev", "t1", "dev-proc", 30*time.Minute)

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
	l := NewLifecycle("dev", "t1", "dev-proc", 0)

	// Can't go from idle to running directly
	err := l.Transition(StateRunning)
	if err == nil {
		t.Error("expected error for idle → running")
	}
}

func TestLifecycle_FailFromStarting(t *testing.T) {
	l := NewLifecycle("dev", "t1", "dev-proc", 0)
	l.Transition(StateStarting)

	if err := l.Transition(StateFailed); err != nil {
		t.Errorf("starting → failed should be valid: %v", err)
	}
}

func TestLifecycle_CancelFromRunning(t *testing.T) {
	l := NewLifecycle("dev", "t1", "dev-proc", 0)
	l.Transition(StateStarting)
	l.Transition(StateRunning)

	if err := l.Transition(StateCancelled); err != nil {
		t.Errorf("running → cancelled should be valid: %v", err)
	}
}

func TestLifecycle_NoTransitionFromTerminal(t *testing.T) {
	l := NewLifecycle("dev", "t1", "dev-proc", 0)
	l.Transition(StateStarting)
	l.Transition(StateRunning)
	l.Transition(StateCompleted)

	err := l.Transition(StateRunning)
	if err == nil {
		t.Error("should not transition from terminal state")
	}
}

func TestLifecycle_CheckTimeout(t *testing.T) {
	l := NewLifecycle("dev", "t1", "dev-proc", 1*time.Millisecond)
	l.Transition(StateStarting)
	l.Transition(StateRunning)

	time.Sleep(2 * time.Millisecond)
	if !l.CheckTimeout() {
		t.Error("expected timeout")
	}
}

func TestLifecycle_NoTimeout(t *testing.T) {
	l := NewLifecycle("dev", "t1", "dev-proc", 10*time.Minute)
	l.Transition(StateStarting)
	l.Transition(StateRunning)

	if l.CheckTimeout() {
		t.Error("should not be timed out")
	}
}

func TestLifecycle_TimeoutDisabled(t *testing.T) {
	l := NewLifecycle("dev", "t1", "dev-proc", 0)
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

func TestRunner_BuildArgs_NonInteractive(t *testing.T) {
	r := NewRunner(nil)

	args := r.BuildArgs(RunConfig{
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

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--print") {
		t.Error("non-interactive should have --print")
	}
	if !strings.Contains(joined, "--output-format stream-json") {
		t.Error("should have stream-json format")
	}
	if !strings.Contains(joined, "--max-turns 30") {
		t.Error("should have max-turns")
	}
	if !strings.Contains(joined, "--permission-mode acceptEdits") {
		t.Error("should have permission-mode")
	}
	if !strings.Contains(joined, "--model sonnet") {
		t.Error("should have model")
	}
	if !strings.Contains(joined, "Implement login API") {
		t.Error("non-interactive should have task prompt as positional argument")
	}
	if strings.Contains(joined, "--prompt") {
		t.Error("should NOT use --prompt flag (claude CLI uses positional argument)")
	}
	if !strings.Contains(joined, "--append-system-prompt test rules") {
		t.Error("should append system prompt from ClaudeMD")
	}
}

func TestRunner_BuildArgs_Interactive(t *testing.T) {
	r := NewRunner(nil)

	args := r.BuildArgs(RunConfig{
		Agent: &config.AgentConfig{
			Name:           "po",
			MaxTurns:       50,
			PermissionMode: "default",
		},
		Global:      &config.Config{},
		Interactive: true,
	})

	joined := strings.Join(args, " ")
	if strings.Contains(joined, "--print") {
		t.Error("interactive should NOT have --print")
	}
	if strings.Contains(joined, "--prompt") {
		t.Error("interactive should NOT have --prompt")
	}
	if !strings.Contains(joined, "--permission-mode default") {
		t.Error("should have permission-mode")
	}
}

func TestRunner_BuildArgs_AllowedTools(t *testing.T) {
	r := NewRunner(nil)

	args := r.BuildArgs(RunConfig{
		Agent: &config.AgentConfig{
			Name:           "dev",
			PermissionMode: "acceptEdits",
			Tools:          []string{"git", "gh", "docker"},
		},
		Global: &config.Config{},
	})

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--allowedTools git,gh,docker") {
		t.Errorf("should have --allowedTools, got: %s", joined)
	}
}

func TestRunner_BuildArgs_DisallowedTools(t *testing.T) {
	r := NewRunner(nil)

	args := r.BuildArgs(RunConfig{
		Agent: &config.AgentConfig{
			Name:            "architect",
			PermissionMode:  "default",
			DisallowedTools: []string{"Edit", "Write", "Bash"},
		},
		Global: &config.Config{},
	})

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--disallowedTools Edit,Write,Bash") {
		t.Errorf("should have --disallowedTools, got: %s", joined)
	}
}

func TestRunner_BuildArgs_BothToolFlags(t *testing.T) {
	r := NewRunner(nil)

	args := r.BuildArgs(RunConfig{
		Agent: &config.AgentConfig{
			Name:            "architect",
			PermissionMode:  "default",
			Tools:           []string{"git", "gh"},
			DisallowedTools: []string{"Edit", "Write"},
		},
		Global: &config.Config{},
	})

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--allowedTools git,gh") {
		t.Errorf("should have --allowedTools, got: %s", joined)
	}
	if !strings.Contains(joined, "--disallowedTools Edit,Write") {
		t.Errorf("should have --disallowedTools, got: %s", joined)
	}
}

func TestRunner_BuildArgs_NoToolFlags(t *testing.T) {
	r := NewRunner(nil)

	args := r.BuildArgs(RunConfig{
		Agent: &config.AgentConfig{
			Name:           "dev",
			PermissionMode: "acceptEdits",
		},
		Global: &config.Config{},
	})

	joined := strings.Join(args, " ")
	if strings.Contains(joined, "--allowedTools") {
		t.Error("should not have --allowedTools when tools is empty")
	}
	if strings.Contains(joined, "--disallowedTools") {
		t.Error("should not have --disallowedTools when disallowedTools is empty")
	}
}

func TestRunner_BuildArgs_NoModel(t *testing.T) {
	r := NewRunner(nil)

	args := r.BuildArgs(RunConfig{
		Agent: &config.AgentConfig{
			Name:           "dev",
			PermissionMode: "acceptEdits",
		},
		Global: &config.Config{},
	})

	joined := strings.Join(args, " ")
	if strings.Contains(joined, "--model") {
		t.Error("should not have --model when not specified")
	}
}

// --- Runner.Start Tests ---

func TestRunner_Start_Success(t *testing.T) {
	mock := &mockExecutor{
		result: &executor.ExecResult{ExitCode: 0, Stdout: "ok"},
	}
	r := NewRunner(mock)

	result, err := r.Start(RunConfig{
		Agent: &config.AgentConfig{
			Name:           "dev",
			MaxTurns:       30,
			PermissionMode: "acceptEdits",
		},
		Global:  &config.Config{},
		WorkDir: "/tmp",
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("exit code = %d, want 0", result.ExitCode)
	}

	// Verify executor received correct config.
	if mock.lastCfg.Command != "claude" {
		t.Errorf("command = %q, want %q", mock.lastCfg.Command, "claude")
	}
	if mock.lastCfg.WorkDir != "/tmp" {
		t.Errorf("workdir = %q, want %q", mock.lastCfg.WorkDir, "/tmp")
	}
	joined := strings.Join(mock.lastCfg.Args, " ")
	if !strings.Contains(joined, "--print") {
		t.Error("headless agent should have --print")
	}
	if !strings.Contains(joined, "--max-turns 30") {
		t.Error("should pass max-turns")
	}
}

func TestRunner_Start_NilAgent(t *testing.T) {
	r := NewRunner(&mockExecutor{})

	_, err := r.Start(RunConfig{
		Global:  &config.Config{},
		WorkDir: "/tmp",
	})
	if err == nil {
		t.Fatal("expected error for nil agent")
	}
}

func TestRunner_Start_NilGlobal(t *testing.T) {
	r := NewRunner(&mockExecutor{})

	_, err := r.Start(RunConfig{
		Agent:   &config.AgentConfig{Name: "dev", PermissionMode: "default"},
		WorkDir: "/tmp",
	})
	if err == nil {
		t.Fatal("expected error for nil global config")
	}
}

func TestRunner_Start_EmptyWorkDir(t *testing.T) {
	r := NewRunner(&mockExecutor{})

	_, err := r.Start(RunConfig{
		Agent:  &config.AgentConfig{Name: "dev", PermissionMode: "default"},
		Global: &config.Config{},
	})
	if err == nil {
		t.Fatal("expected error for empty work directory")
	}
}

// --- BuildTaskPrompt Tests ---

func TestBuildTaskPrompt(t *testing.T) {
	prompt := BuildTaskPrompt("백엔드 개발자", "backend-dev", "task-001", ".pylon/runtime/inbox")

	if !strings.Contains(prompt, "백엔드 개발자") {
		t.Error("prompt should contain role")
	}
	if !strings.Contains(prompt, "backend-dev") {
		t.Error("prompt should contain agent name")
	}
	if !strings.Contains(prompt, "task-001.task.json") {
		t.Error("prompt should contain task file path")
	}
	if !strings.Contains(prompt, "inbox") {
		t.Error("prompt should reference inbox")
	}
	if !strings.Contains(prompt, "outbox") {
		t.Error("prompt should reference outbox")
	}
}

// --- PrepareWorkDir Tests ---

func TestPrepareWorkDir_NilManager(t *testing.T) {
	workDir, cleanup, err := PrepareWorkDir(nil, "worktree", "branch", "/project", "dev")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if workDir != "/project" {
		t.Errorf("workDir = %q, want /project", workDir)
	}
	if cleanup == nil {
		t.Fatal("cleanup should not be nil")
	}
	if err := cleanup(); err != nil {
		t.Errorf("cleanup error: %v", err)
	}
}

func TestPrepareWorkDir_NoWorktreeIsolation(t *testing.T) {
	workDir, _, err := PrepareWorkDir(nil, "process", "branch", "/project", "dev")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if workDir != "/project" {
		t.Errorf("workDir = %q, want /project", workDir)
	}
}

func TestPrepareWorkDir_DisabledManager(t *testing.T) {
	wm := &git.WorktreeManager{Enabled: false}
	workDir, _, err := PrepareWorkDir(wm, "worktree", "branch", "/project", "dev")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if workDir != "/project" {
		t.Errorf("workDir = %q, want /project", workDir)
	}
}
