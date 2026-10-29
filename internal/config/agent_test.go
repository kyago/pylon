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
		{"description", agent.Description, "Implements project-api backend features following Go standard project layout"},
		{"role", agent.Role, "Backend Developer"},
		{"backend", agent.Backend, "claude-code"},
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
	if agent.Description != "Read-only solution architect that produces design documents and ADRs" {
		t.Errorf("description: got %q", agent.Description)
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
	if agent.Description != "" {
		t.Errorf("description should be empty, got %q", agent.Description)
	}
	if agent.Backend != "" {
		t.Errorf("backend should be empty, got %q", agent.Backend)
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

func TestParseAgentDataProviderAndCapabilities(t *testing.T) {
	data := []byte(`---
name: verifier
role: Verifier
provider: auto
requiredCapabilities:
  - read_files
  - run_shell
---

# Verifier
`)
	agent, err := ParseAgentData(data)
	if err != nil {
		t.Fatal(err)
	}
	providerName, deprecated := agent.EffectiveProvider()
	if providerName != "auto" || deprecated {
		t.Fatalf("effective provider = %q, deprecated=%v", providerName, deprecated)
	}
	capabilities, err := agent.RequiredCapabilitySet()
	if err != nil {
		t.Fatal(err)
	}
	if !capabilities.ReadFiles || !capabilities.RunShell || capabilities.EditFiles {
		t.Fatalf("capabilities = %+v", capabilities)
	}
}

func TestDecisionEvaluatorPolicyRejectsWriteAndShell(t *testing.T) {
	data := []byte(`---
name: verifier
role: Verifier
evaluationRole: decision
accessMode: read_only
inputPolicy: isolated_evidence
tools: [Read, Bash]
---
Verifier
`)
	if _, err := ParseAgentData(data); err == nil {
		t.Fatal("decision evaluator allowing Bash was accepted")
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
