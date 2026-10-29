package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/kyago/pylon/internal/provider"
	"gopkg.in/yaml.v3"
)

// AgentConfig represents an agent definition parsed from a .md file.
// Spec Reference: Section 5 "Agent Configuration Format"
type AgentConfig struct {
	// YAML frontmatter fields (between --- delimiters)
	Name                 string   `yaml:"name"`
	Description          string   `yaml:"description"`
	Role                 string   `yaml:"role"`
	Type                 string   `yaml:"type"`
	Provider             string   `yaml:"provider,omitempty"`
	Backend              string   `yaml:"backend,omitempty"`
	RequiredCapabilities []string `yaml:"requiredCapabilities,omitempty"`
	EvaluationRole       string   `yaml:"evaluationRole,omitempty"`
	AccessMode           string   `yaml:"accessMode,omitempty"`
	InputPolicy          string   `yaml:"inputPolicy,omitempty"`
	Scope                []string `yaml:"scope"`
	Tools                []string `yaml:"tools"`
	DisallowedTools      []string `yaml:"disallowedTools"`
	Domain               string   `yaml:"domain"`
	Skills               []string `yaml:"skills"`

	// Markdown body (everything after the second ---)
	Body string `yaml:"-"`
	// Source file path (for debugging)
	FilePath string `yaml:"-"`
}

const (
	EvaluationRoleDecision      = "decision"
	EvaluationRoleInvestigation = "investigation"
	AccessModeStandard          = "standard"
	AccessModeReadOnly          = "read_only"
	InputPolicyFullContext      = "full_context"
	InputPolicyScopedContext    = "scoped_context"
	InputPolicyIsolatedEvidence = "isolated_evidence"
)

func (a AgentConfig) EffectiveProvider() (string, bool) {
	if providerName := strings.TrimSpace(a.Provider); providerName != "" {
		return providerName, false
	}
	if backendName := strings.TrimSpace(a.Backend); backendName != "" {
		return backendName, true
	}
	return "auto", false
}

func (a AgentConfig) RequiredCapabilitySet() (provider.CapabilitySet, error) {
	return provider.ParseCapabilities(a.RequiredCapabilities)
}

// ParseAgentFile reads a .md file and extracts YAML frontmatter + markdown body.
// Spec Reference: Section 5 "Agent Configuration Format"
//
// Parsing logic:
//  1. First line must be "---"
//  2. YAML content until second "---"
//  3. Everything after second "---" is the markdown body
//  4. Required fields: name, role
func ParseAgentFile(path string) (*AgentConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read agent file %s: %w", path, err)
	}

	agent, err := ParseAgentData(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse agent file %s: %w", path, err)
	}
	agent.FilePath = path
	return agent, nil
}

// ParseAgentData parses agent configuration from raw bytes.
func ParseAgentData(data []byte) (*AgentConfig, error) {
	content := string(data)
	frontmatter, body, err := splitFrontmatter(content)
	if err != nil {
		return nil, err
	}

	agent := &AgentConfig{}
	if err := yaml.Unmarshal([]byte(frontmatter), agent); err != nil {
		return nil, fmt.Errorf("failed to parse frontmatter YAML: %w", err)
	}

	agent.Body = body

	// Validate required fields
	if agent.Name == "" {
		return nil, fmt.Errorf("agent validation error: 'name' is required")
	}
	if agent.Role == "" {
		return nil, fmt.Errorf("agent validation error: 'role' is required")
	}
	if err := agent.ValidateEvaluationPolicy(); err != nil {
		return nil, err
	}

	return agent, nil
}

// splitFrontmatter separates YAML frontmatter from markdown body.
// Returns (frontmatter, body, error).
func splitFrontmatter(content string) (string, string, error) {
	// Normalize line endings
	content = strings.ReplaceAll(content, "\r\n", "\n")

	// Must start with ---
	if !strings.HasPrefix(content, "---") {
		return "", "", fmt.Errorf("agent file must start with '---' (YAML frontmatter delimiter)")
	}

	// Find the second ---
	rest := content[3:] // skip first ---
	// Skip the newline after first ---
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	}

	idx := strings.Index(rest, "\n---")
	if idx == -1 {
		return "", "", fmt.Errorf("missing closing '---' for YAML frontmatter")
	}

	frontmatter := rest[:idx]
	body := ""

	// Everything after the closing --- is the body
	remaining := rest[idx+4:] // skip "\n---"
	if len(remaining) > 0 && remaining[0] == '\n' {
		remaining = remaining[1:]
	}
	body = strings.TrimRight(remaining, "\n")

	return frontmatter, body, nil
}

func (a AgentConfig) CanIssueFinalVerdict() bool {
	return a.EvaluationRole == EvaluationRoleDecision
}

func (a AgentConfig) ValidateEvaluationPolicy() error {
	switch a.EvaluationRole {
	case "", EvaluationRoleDecision, EvaluationRoleInvestigation:
	default:
		return fmt.Errorf("agent validation error: unknown evaluationRole %q", a.EvaluationRole)
	}
	switch a.AccessMode {
	case "", AccessModeStandard, AccessModeReadOnly:
	default:
		return fmt.Errorf("agent validation error: unknown accessMode %q", a.AccessMode)
	}
	switch a.InputPolicy {
	case "", InputPolicyFullContext, InputPolicyScopedContext, InputPolicyIsolatedEvidence:
	default:
		return fmt.Errorf("agent validation error: unknown inputPolicy %q", a.InputPolicy)
	}
	if a.EvaluationRole != EvaluationRoleDecision {
		return nil
	}
	if a.AccessMode != "" && a.AccessMode != AccessModeReadOnly {
		return errors.New("agent validation error: decision evaluators must use read_only accessMode")
	}
	if a.InputPolicy != "" && a.InputPolicy != InputPolicyIsolatedEvidence {
		return errors.New("agent validation error: decision evaluators must use isolated_evidence inputPolicy")
	}
	for _, capability := range a.RequiredCapabilities {
		if capability == string(provider.CapabilityEditFiles) || capability == string(provider.CapabilityRunShell) {
			return fmt.Errorf("agent validation error: decision evaluator cannot require %s", capability)
		}
	}
	for _, tool := range a.Tools {
		switch strings.ToLower(tool) {
		case "bash", "shell", "edit", "write", "notebookedit":
			return fmt.Errorf("agent validation error: decision evaluator cannot allow %s", tool)
		}
	}
	return nil
}
