package config

import (
	"path/filepath"
	"testing"
)

func TestParseAgentFile_FullAgent(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "agents", "backend-dev.md")
	agent, err := ParseAgentFile(path)
	if err != nil {
		t.Fatalf("ParseAgentFile failed: %v", err)
	}

	tests := []struct {
		name     string
		got      interface{}
		expected interface{}
	}{
		{"name", agent.Name, "backend-dev"},
		{"role", agent.Role, "Backend Developer"},
		{"backend", agent.Backend, "claude-code"},
		{"maxTurns", agent.MaxTurns, 30},
		{"permissionMode", agent.PermissionMode, "acceptEdits"},
		{"isolation", agent.Isolation, "worktree"},
		{"model", agent.Model, "sonnet"},
		{"filePath", agent.FilePath, path},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("got %v, expected %v", tt.got, tt.expected)
			}
		})
	}

	// Check scope
	if len(agent.Scope) != 1 || agent.Scope[0] != "project-api" {
		t.Errorf("expected scope [project-api], got %v", agent.Scope)
	}

	// Check tools
	expectedTools := []string{"git", "gh", "docker"}
	if len(agent.Tools) != len(expectedTools) {
		t.Fatalf("expected %d tools, got %d", len(expectedTools), len(agent.Tools))
	}
	for i, tool := range expectedTools {
		if agent.Tools[i] != tool {
			t.Errorf("tool[%d]: got %q, expected %q", i, agent.Tools[i], tool)
		}
	}

	// Check env
	if v, ok := agent.Env["CLAUDE_CODE_EFFORT_LEVEL"]; !ok || v != "high" {
		t.Errorf("expected env CLAUDE_CODE_EFFORT_LEVEL=high, got %v", agent.Env)
	}

	// Check body contains expected content
	if agent.Body == "" {
		t.Error("expected non-empty body")
	}
	if !contains(agent.Body, "Backend Developer") {
		t.Error("body should contain 'Backend Developer'")
	}
	if !contains(agent.Body, "Go standard project layout") {
		t.Error("body should contain 'Go standard project layout'")
	}
}

func TestParseAgentFile_ArchitectAgent(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "agents", "architect.md")
	agent, err := ParseAgentFile(path)
	if err != nil {
		t.Fatalf("ParseAgentFile failed: %v", err)
	}

	if agent.Name != "architect" {
		t.Errorf("name: got %q, expected %q", agent.Name, "architect")
	}
	if agent.Role != "Solution Architect" {
		t.Errorf("role: got %q, expected %q", agent.Role, "Solution Architect")
	}

	// Check tools (allowedTools)
	expectedTools := []string{"git", "gh"}
	if len(agent.Tools) != len(expectedTools) {
		t.Fatalf("expected %d tools, got %d", len(expectedTools), len(agent.Tools))
	}
	for i, tool := range expectedTools {
		if agent.Tools[i] != tool {
			t.Errorf("tool[%d]: got %q, expected %q", i, agent.Tools[i], tool)
		}
	}

	// Check disallowedTools
	expectedDisallowed := []string{"Edit", "Write", "Bash"}
	if len(agent.DisallowedTools) != len(expectedDisallowed) {
		t.Fatalf("expected %d disallowedTools, got %d", len(expectedDisallowed), len(agent.DisallowedTools))
	}
	for i, tool := range expectedDisallowed {
		if agent.DisallowedTools[i] != tool {
			t.Errorf("disallowedTools[%d]: got %q, expected %q", i, agent.DisallowedTools[i], tool)
		}
	}
}

func TestParseAgentData_DisallowedTools(t *testing.T) {
	data := []byte(`---
name: readonly-agent
role: Read Only Agent
disallowedTools:
  - Edit
  - Write
---
Read-only agent body.
`)
	agent, err := ParseAgentData(data)
	if err != nil {
		t.Fatalf("ParseAgentData failed: %v", err)
	}

	if len(agent.DisallowedTools) != 2 {
		t.Fatalf("expected 2 disallowedTools, got %d", len(agent.DisallowedTools))
	}
	if agent.DisallowedTools[0] != "Edit" || agent.DisallowedTools[1] != "Write" {
		t.Errorf("unexpected disallowedTools: %v", agent.DisallowedTools)
	}

	// Tools (allowed) should be empty
	if len(agent.Tools) != 0 {
		t.Errorf("expected empty tools, got %v", agent.Tools)
	}
}

func TestParseAgentFile_MinimalAgent(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "agents", "minimal-agent.md")
	agent, err := ParseAgentFile(path)
	if err != nil {
		t.Fatalf("ParseAgentFile failed: %v", err)
	}

	if agent.Name != "simple" {
		t.Errorf("name: got %q, expected %q", agent.Name, "simple")
	}
	if agent.Role != "Simple Agent" {
		t.Errorf("role: got %q, expected %q", agent.Role, "Simple Agent")
	}

	// Optional fields should be zero-valued
	if agent.Backend != "" {
		t.Errorf("backend should be empty, got %q", agent.Backend)
	}
	if agent.MaxTurns != 0 {
		t.Errorf("maxTurns should be 0, got %d", agent.MaxTurns)
	}
	if agent.PermissionMode != "" {
		t.Errorf("permissionMode should be empty, got %q", agent.PermissionMode)
	}
	if len(agent.Scope) != 0 {
		t.Errorf("scope should be empty, got %v", agent.Scope)
	}

	// Body should be present
	if agent.Body == "" {
		t.Error("expected non-empty body")
	}
}

func TestParseAgentFile_MissingName(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "agents", "missing-name.md")
	_, err := ParseAgentFile(path)
	if err == nil {
		t.Fatal("expected error for missing name, got nil")
	}
}

func TestParseAgentFile_MissingRole(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "agents", "missing-role.md")
	_, err := ParseAgentFile(path)
	if err == nil {
		t.Fatal("expected error for missing role, got nil")
	}
}

func TestParseAgentFile_NoFrontmatter(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "agents", "no-frontmatter.md")
	_, err := ParseAgentFile(path)
	if err == nil {
		t.Fatal("expected error for missing frontmatter, got nil")
	}
}

func TestParseAgentFile_FileNotFound(t *testing.T) {
	_, err := ParseAgentFile("/nonexistent/path/agent.md")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}

func TestParseAgentData_FrontmatterOnly(t *testing.T) {
	data := []byte(`---
name: test-agent
role: Test Agent
---
`)
	agent, err := ParseAgentData(data)
	if err != nil {
		t.Fatalf("ParseAgentData failed: %v", err)
	}

	if agent.Name != "test-agent" {
		t.Errorf("name: got %q, expected %q", agent.Name, "test-agent")
	}
	if agent.Role != "Test Agent" {
		t.Errorf("role: got %q, expected %q", agent.Role, "Test Agent")
	}
}

func TestParseAgentData_MissingClosingDelimiter(t *testing.T) {
	data := []byte(`---
name: broken
role: Broken Agent
# no closing ---
`)
	_, err := ParseAgentData(data)
	if err == nil {
		t.Fatal("expected error for missing closing delimiter, got nil")
	}
}

func TestAgentConfig_ResolveDefaults(t *testing.T) {
	cfg := &Config{
		Runtime: RuntimeConfig{
			Backend:        "claude-code",
			MaxTurns:       50,
			PermissionMode: "acceptEdits",
			Env: map[string]string{
				"CLAUDE_AUTOCOMPACT_PCT_OVERRIDE": "80",
				"CLAUDE_CODE_EFFORT_LEVEL":        "high",
			},
		},
	}

	tests := []struct {
		name     string
		agent    *AgentConfig
		checkFn  func(*AgentConfig) bool
		expected string
	}{
		{
			name:  "inherits backend from config",
			agent: &AgentConfig{Name: "test", Role: "Test"},
			checkFn: func(a *AgentConfig) bool {
				return a.Backend == "claude-code"
			},
			expected: "backend should be claude-code",
		},
		{
			name:  "inherits maxTurns from config",
			agent: &AgentConfig{Name: "test", Role: "Test"},
			checkFn: func(a *AgentConfig) bool {
				return a.MaxTurns == 50
			},
			expected: "maxTurns should be 50",
		},
		{
			name:  "inherits permissionMode from config",
			agent: &AgentConfig{Name: "test", Role: "Test"},
			checkFn: func(a *AgentConfig) bool {
				return a.PermissionMode == "acceptEdits"
			},
			expected: "permissionMode should be acceptEdits",
		},
		{
			name:  "defaults isolation to worktree",
			agent: &AgentConfig{Name: "test", Role: "Test"},
			checkFn: func(a *AgentConfig) bool {
				return a.Isolation == "worktree"
			},
			expected: "isolation should be worktree",
		},
		{
			name:  "agent backend overrides config",
			agent: &AgentConfig{Name: "test", Role: "Test", Backend: "openai"},
			checkFn: func(a *AgentConfig) bool {
				return a.Backend == "openai"
			},
			expected: "backend should remain openai",
		},
		{
			name:  "agent maxTurns overrides config",
			agent: &AgentConfig{Name: "test", Role: "Test", MaxTurns: 30},
			checkFn: func(a *AgentConfig) bool {
				return a.MaxTurns == 30
			},
			expected: "maxTurns should remain 30",
		},
		{
			name:  "agent permissionMode overrides config",
			agent: &AgentConfig{Name: "test", Role: "Test", PermissionMode: "default"},
			checkFn: func(a *AgentConfig) bool {
				return a.PermissionMode == "default"
			},
			expected: "permissionMode should remain default",
		},
		{
			name:  "agent isolation overrides default",
			agent: &AgentConfig{Name: "test", Role: "Test", Isolation: "none"},
			checkFn: func(a *AgentConfig) bool {
				return a.Isolation == "none"
			},
			expected: "isolation should remain none",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.agent.ResolveDefaults(cfg)
			if !tt.checkFn(tt.agent) {
				t.Error(tt.expected)
			}
		})
	}
}

func TestInferAgentType(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{"backend-dev", "dev"},
		{"frontend-dev", "dev"},
		{"fullstack", "dev"},
		{"architect", ""},
		{"pm", ""},
		{"po", ""},
		{"my-custom-agent", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InferAgentType(tt.name)
			if got != tt.expected {
				t.Errorf("InferAgentType(%q) = %q, want %q", tt.name, got, tt.expected)
			}
		})
	}
}

func TestAgentConfig_ResolveDefaults_TypeInference(t *testing.T) {
	cfg := &Config{
		Runtime: RuntimeConfig{
			Backend:        "claude-code",
			MaxTurns:       50,
			PermissionMode: "acceptEdits",
		},
	}

	// Type is inferred from name
	agent := &AgentConfig{Name: "backend-dev", Role: "Backend Dev"}
	agent.ResolveDefaults(cfg)
	if agent.Type != "dev" {
		t.Errorf("expected inferred type 'dev', got %q", agent.Type)
	}

	// Explicit type is preserved
	agent2 := &AgentConfig{Name: "backend-dev", Role: "Backend Dev", Type: "infra"}
	agent2.ResolveDefaults(cfg)
	if agent2.Type != "infra" {
		t.Errorf("expected explicit type 'infra' to be preserved, got %q", agent2.Type)
	}

	// Unknown name → empty type
	agent3 := &AgentConfig{Name: "custom-agent", Role: "Custom"}
	agent3.ResolveDefaults(cfg)
	if agent3.Type != "" {
		t.Errorf("expected empty type for unknown name, got %q", agent3.Type)
	}
}

func TestParseAgentData_TypeField(t *testing.T) {
	data := []byte(`---
name: my-dev
role: Developer
type: dev
---
Custom dev agent.
`)
	agent, err := ParseAgentData(data)
	if err != nil {
		t.Fatalf("ParseAgentData failed: %v", err)
	}
	if agent.Type != "dev" {
		t.Errorf("expected type 'dev', got %q", agent.Type)
	}
}

func TestAgentConfig_ResolveDefaults_EnvMerge(t *testing.T) {
	cfg := &Config{
		Runtime: RuntimeConfig{
			Backend:        "claude-code",
			MaxTurns:       50,
			PermissionMode: "acceptEdits",
			Env: map[string]string{
				"CLAUDE_AUTOCOMPACT_PCT_OVERRIDE": "80",
				"CLAUDE_CODE_EFFORT_LEVEL":        "high",
				"GLOBAL_VAR":                      "global",
			},
		},
	}

	agent := &AgentConfig{
		Name: "test",
		Role: "Test",
		Env: map[string]string{
			"CLAUDE_CODE_EFFORT_LEVEL": "low",  // Override global
			"AGENT_VAR":               "agent", // Agent-specific
		},
	}

	agent.ResolveDefaults(cfg)

	// Agent env should override global env
	if v := agent.Env["CLAUDE_CODE_EFFORT_LEVEL"]; v != "low" {
		t.Errorf("expected agent override to win, got %q", v)
	}

	// Global env should be present if not overridden
	if v := agent.Env["CLAUDE_AUTOCOMPACT_PCT_OVERRIDE"]; v != "80" {
		t.Errorf("expected global env to persist, got %q", v)
	}
	if v := agent.Env["GLOBAL_VAR"]; v != "global" {
		t.Errorf("expected global var to persist, got %q", v)
	}

	// Agent-specific env should be present
	if v := agent.Env["AGENT_VAR"]; v != "agent" {
		t.Errorf("expected agent var to persist, got %q", v)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsStr(s, substr)
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
