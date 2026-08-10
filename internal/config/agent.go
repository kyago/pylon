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
	Name                 string            `yaml:"name"`
	Description          string            `yaml:"description"`
	Role                 string            `yaml:"role"`
	Type                 string            `yaml:"type"`
	Provider             string            `yaml:"provider,omitempty"`
	Backend              string            `yaml:"backend,omitempty"`
	RequiredCapabilities []string          `yaml:"requiredCapabilities,omitempty"`
	EvaluationRole       string            `yaml:"evaluationRole,omitempty"`
	AccessMode           string            `yaml:"accessMode,omitempty"`
	InputPolicy          string            `yaml:"inputPolicy,omitempty"`
	Scope                []string          `yaml:"scope"`
	Tools                []string          `yaml:"tools"`
	DisallowedTools      []string          `yaml:"disallowedTools"`
	MaxTurns             int               `yaml:"maxTurns"`
	PermissionMode       string            `yaml:"permissionMode"`
	Isolation            string            `yaml:"isolation"`
	Model                string            `yaml:"model"`
	Timeout              string            `yaml:"timeout"`
	Env                  map[string]string `yaml:"env"`
	Domain               string            `yaml:"domain"`
	Skills               []string          `yaml:"skills"`

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

// InferAgentType infers the agent type from the agent name.
// Known dev agent names map to "dev" type for backward compatibility
// with configurations that don't specify the type field explicitly.
func InferAgentType(name string) string {
	devNames := map[string]bool{
		"backend-dev":  true,
		"frontend-dev": true,
		"fullstack":    true,
	}
	if devNames[name] {
		return "dev"
	}
	return ""
}

func InferAgentEvaluationPolicy(name string) (role, accessMode, inputPolicy string) {
	decisionAgents := map[string]bool{
		"verifier":          true,
		"critic":            true,
		"code-reviewer":     true,
		"security-reviewer": true,
		"fact-checker":      true,
		"content-reviewer":  true,
	}
	if decisionAgents[name] {
		return EvaluationRoleDecision, AccessModeReadOnly, InputPolicyIsolatedEvidence
	}
	investigationAgents := map[string]bool{
		"explorer":       true,
		"tracer":         true,
		"analyst":        true,
		"researcher":     true,
		"doc-specialist": true,
	}
	if investigationAgents[name] {
		return EvaluationRoleInvestigation, AccessModeReadOnly, InputPolicyScopedContext
	}
	return "", AccessModeStandard, InputPolicyFullContext
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

// ResolveDefaults fills in missing agent fields with values from the global config.
// Spec Reference: Section 5 "frontmatter field spec" - default value inheritance
//
// Inheritance rules:
//   - type <- InferAgentType(name) if empty
//   - provider <- config.yml runtime.provider or deprecated runtime.backend
//   - maxTurns <- config.yml runtime.max_turns
//   - permissionMode <- config.yml runtime.permission_mode
//   - isolation <- "worktree" (hardcoded default)
//   - env <- config.yml runtime.env merged with agent env (agent takes precedence)
func (a *AgentConfig) ResolveDefaults(cfg *Config) {
	if a.Type == "" {
		a.Type = InferAgentType(a.Name)
	}
	role, accessMode, inputPolicy := InferAgentEvaluationPolicy(a.Name)
	if a.EvaluationRole == "" {
		a.EvaluationRole = role
	}
	if a.AccessMode == "" {
		a.AccessMode = accessMode
	}
	if a.InputPolicy == "" {
		a.InputPolicy = inputPolicy
	}
	if a.EvaluationRole == EvaluationRoleDecision {
		a.RequiredCapabilities = removeCapabilityNames(a.RequiredCapabilities, []string{
			string(provider.CapabilityEditFiles),
			string(provider.CapabilityRunShell),
			string(provider.CapabilitySpawnSubagents),
			string(provider.CapabilityBackgroundExecution),
		})
		a.RequiredCapabilities = mergeCapabilityNames(a.RequiredCapabilities, []string{
			string(provider.CapabilityReadFiles),
			string(provider.CapabilityStructuredOutput),
			string(provider.CapabilityToolRestrictions),
		})
	}
	if a.Provider == "" && a.Backend == "" {
		a.Provider, _ = cfg.Runtime.EffectiveProvider()
	}
	if a.MaxTurns == 0 {
		a.MaxTurns = cfg.Runtime.MaxTurns
	}
	if a.PermissionMode == "" {
		a.PermissionMode = cfg.Runtime.PermissionMode
	}
	if a.Isolation == "" {
		a.Isolation = "worktree"
	}
	if a.EvaluationRole == EvaluationRoleDecision {
		a.PermissionMode = "default"
		a.Isolation = "readonly"
		a.Tools = filterReadOnlyTools(a.Tools)
		if len(a.Tools) == 0 {
			a.Tools = []string{"Read", "Grep", "Glob"}
		}
		a.DisallowedTools = mergeCapabilityNames(a.DisallowedTools, []string{"Edit", "Write", "NotebookEdit", "Bash"})
	}

	// Merge env: config.yml runtime.env as base, agent env overrides
	if cfg.Runtime.Env != nil || a.Env != nil {
		merged := make(map[string]string)
		for k, v := range cfg.Runtime.Env {
			merged[k] = v
		}
		for k, v := range a.Env {
			merged[k] = v
		}
		a.Env = merged
	}
}

func removeCapabilityNames(existing, denied []string) []string {
	blocked := make(map[string]struct{}, len(denied))
	for _, name := range denied {
		blocked[name] = struct{}{}
	}
	filtered := make([]string, 0, len(existing))
	for _, name := range existing {
		if _, denied := blocked[name]; !denied {
			filtered = append(filtered, name)
		}
	}
	return filtered
}

func filterReadOnlyTools(tools []string) []string {
	filtered := make([]string, 0, len(tools))
	for _, tool := range tools {
		switch strings.ToLower(tool) {
		case "bash", "shell", "edit", "write", "notebookedit":
			continue
		default:
			filtered = append(filtered, tool)
		}
	}
	return filtered
}

func mergeCapabilityNames(existing, required []string) []string {
	seen := make(map[string]struct{}, len(existing)+len(required))
	merged := make([]string, 0, len(existing)+len(required))
	for _, name := range append(append([]string(nil), existing...), required...) {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		merged = append(merged, name)
	}
	return merged
}
