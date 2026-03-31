package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// AgentConfig represents an agent definition parsed from a .md file.
// Spec Reference: Section 5 "Agent Configuration Format"
type AgentConfig struct {
	// YAML frontmatter fields (between --- delimiters)
	Name           string            `yaml:"name"`
	Role           string            `yaml:"role"`
	Type           string            `yaml:"type"`
	Backend        string            `yaml:"backend"`
	Scope          []string          `yaml:"scope"`
	Tools          []string          `yaml:"tools"`
	DisallowedTools []string          `yaml:"disallowedTools"`
	MaxTurns       int               `yaml:"maxTurns"`
	PermissionMode string            `yaml:"permissionMode"`
	Isolation      string            `yaml:"isolation"`
	Model          string            `yaml:"model"`
	Timeout        string            `yaml:"timeout"`
	Env            map[string]string `yaml:"env"`
	Domain         string            `yaml:"domain"`
	Skills         []string          `yaml:"skills"`

	// Markdown body (everything after the second ---)
	Body string `yaml:"-"`
	// Source file path (for debugging)
	FilePath string `yaml:"-"`
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

// ResolveDefaults fills in missing agent fields with values from the global config.
// Spec Reference: Section 5 "frontmatter field spec" - default value inheritance
//
// Inheritance rules:
//   - type <- InferAgentType(name) if empty
//   - backend <- config.yml runtime.backend
//   - maxTurns <- config.yml runtime.max_turns
//   - permissionMode <- config.yml runtime.permission_mode
//   - isolation <- "worktree" (hardcoded default)
//   - env <- config.yml runtime.env merged with agent env (agent takes precedence)
func (a *AgentConfig) ResolveDefaults(cfg *Config) {
	if a.Type == "" {
		a.Type = InferAgentType(a.Name)
	}
	if a.Backend == "" {
		a.Backend = cfg.Runtime.Backend
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
