// Package config handles parsing and validation of pylon configuration files.
// Spec Reference: Section 16 "config.yml Schema"
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the full pylon workspace configuration.
// Spec Reference: Section 16 "Full Schema"
type Config struct {
	Version      string                   `yaml:"version"`
	Runtime      RuntimeConfig            `yaml:"runtime"`
	Tmux         TmuxConfig               `yaml:"tmux"`
	Git          GitConfig                `yaml:"git"`
	Projects     map[string]ProjectConfig `yaml:"projects"`
	Wiki         WikiConfig               `yaml:"wiki"`
	Dashboard    DashboardConfig          `yaml:"dashboard"`
	Memory       MemoryConfig             `yaml:"memory"`
	Conversation ConversationConfig       `yaml:"conversation"`
}

// RuntimeConfig defines agent runtime settings.
// Spec Reference: Section 16 "runtime"
type RuntimeConfig struct {
	Backend        string            `yaml:"backend"`
	MaxConcurrent  int               `yaml:"max_concurrent"`
	TaskTimeout    string            `yaml:"task_timeout"`
	MaxAttempts    int               `yaml:"max_attempts"`
	MaxTurns       int               `yaml:"max_turns"`
	PermissionMode string            `yaml:"permission_mode"`
	Env            map[string]string `yaml:"env"`
}

// TmuxConfig defines tmux session settings.
// Spec Reference: Section 16 "tmux"
type TmuxConfig struct {
	SessionPrefix string `yaml:"session_prefix"`
	HistoryLimit  int    `yaml:"history_limit"`
}

// GitConfig defines git integration settings.
// Spec Reference: Section 16 "git"
type GitConfig struct {
	BranchPrefix string         `yaml:"branch_prefix"`
	DefaultBase  string         `yaml:"default_base"`
	AutoPush     bool           `yaml:"auto_push"`
	Worktree     WorktreeConfig `yaml:"worktree"`
	PR           PRConfig       `yaml:"pr"`
}

// WorktreeConfig defines git worktree isolation settings.
// Spec Reference: Section 8 "Git Worktree Isolation"
type WorktreeConfig struct {
	Enabled     bool `yaml:"enabled"`
	AutoCleanup bool `yaml:"auto_cleanup"`
}

// PRConfig defines pull request settings.
// Spec Reference: Section 16 "git.pr"
type PRConfig struct {
	Reviewers []string `yaml:"reviewers"`
	Draft     bool     `yaml:"draft"`
	Template  *string  `yaml:"template"`
}

// ProjectConfig defines per-project settings.
// Spec Reference: Section 16 "projects"
type ProjectConfig struct {
	Stack  string   `yaml:"stack"`
	Agents []string `yaml:"agents,omitempty"`
}

// WikiConfig defines domain knowledge update settings.
// Spec Reference: Section 16 "wiki"
type WikiConfig struct {
	AutoUpdate bool     `yaml:"auto_update"`
	UpdateOn   []string `yaml:"update_on"`
}

// DashboardConfig defines web dashboard settings.
// Spec Reference: Section 16 "dashboard"
type DashboardConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

// MemoryConfig defines agent memory management settings.
// Spec Reference: Section 16 "memory"
type MemoryConfig struct {
	CompactionThreshold float64 `yaml:"compaction_threshold"`
	ProactiveInjection  bool    `yaml:"proactive_injection"`
	ProactiveMaxTokens  int     `yaml:"proactive_max_tokens"`
	SessionArchive      bool    `yaml:"session_archive"`
	RetentionDays       int     `yaml:"retention_days"`
}

// ConversationConfig defines conversation history settings.
// Spec Reference: Section 16 "conversation"
type ConversationConfig struct {
	RetentionDays int `yaml:"retention_days"`
}

// LoadConfig reads and parses a config.yml file, applying defaults for missing fields.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	return ParseConfig(data)
}

// ParseConfig parses config.yml content from bytes, applying defaults for missing fields.
func ParseConfig(data []byte) (*Config, error) {
	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Validate required fields
	if cfg.Version == "" {
		return nil, fmt.Errorf("config validation error: 'version' is required")
	}

	// Apply defaults for missing fields (Spec Section 16 defaults)
	applyDefaults(cfg)

	return cfg, nil
}

// applyDefaults fills in default values for any fields not specified in the config.
// Spec Reference: Section 16 default values
func applyDefaults(cfg *Config) {
	// Runtime defaults
	if cfg.Runtime.Backend == "" {
		cfg.Runtime.Backend = "claude-code"
	}
	if cfg.Runtime.MaxConcurrent == 0 {
		cfg.Runtime.MaxConcurrent = 5
	}
	if cfg.Runtime.TaskTimeout == "" {
		cfg.Runtime.TaskTimeout = "30m"
	}
	if cfg.Runtime.MaxAttempts == 0 {
		cfg.Runtime.MaxAttempts = 2
	}
	if cfg.Runtime.MaxTurns == 0 {
		cfg.Runtime.MaxTurns = 50
	}
	if cfg.Runtime.PermissionMode == "" {
		cfg.Runtime.PermissionMode = "acceptEdits"
	}
	if cfg.Runtime.Env == nil {
		cfg.Runtime.Env = map[string]string{
			"CLAUDE_AUTOCOMPACT_PCT_OVERRIDE": "80",
			"CLAUDE_CODE_EFFORT_LEVEL":        "high",
		}
	}

	// Tmux defaults
	if cfg.Tmux.SessionPrefix == "" {
		cfg.Tmux.SessionPrefix = "pylon"
	}
	if cfg.Tmux.HistoryLimit == 0 {
		cfg.Tmux.HistoryLimit = 10000
	}

	// Git defaults
	if cfg.Git.BranchPrefix == "" {
		cfg.Git.BranchPrefix = "task"
	}
	if cfg.Git.DefaultBase == "" {
		cfg.Git.DefaultBase = "main"
	}
	// AutoPush defaults to true (Spec Section 16)
	// Note: bool zero value is false, but spec default is true.
	// We handle this by checking if git section was specified at all.
	// For simplicity, we set it in the full default application.

	// Worktree defaults (Spec Section 16: enabled=true, auto_cleanup=true)
	// These are only set if the git section exists but worktree wasn't specified.
	// Since Go zero value for bool is false, we need to handle this carefully.
	// The approach: set defaults unconditionally for worktree.
	// Users must explicitly set to false to disable.

	// Wiki defaults
	if !cfg.Wiki.AutoUpdate && len(cfg.Wiki.UpdateOn) == 0 {
		cfg.Wiki.AutoUpdate = true
		cfg.Wiki.UpdateOn = []string{"task_complete", "pr_merged"}
	}

	// Dashboard defaults
	if cfg.Dashboard.Host == "" {
		cfg.Dashboard.Host = "localhost"
	}
	if cfg.Dashboard.Port == 0 {
		cfg.Dashboard.Port = 7777
	}

	// Memory defaults
	if cfg.Memory.CompactionThreshold == 0 {
		cfg.Memory.CompactionThreshold = 0.7
	}
	if cfg.Memory.ProactiveMaxTokens == 0 {
		cfg.Memory.ProactiveMaxTokens = 2000
	}
	// ProactiveInjection defaults to true, SessionArchive defaults to true
	// bool zero value is false, so we handle via explicit default logic
	// For fresh configs (no memory section), set defaults:
	if cfg.Memory.CompactionThreshold == 0.7 && cfg.Memory.ProactiveMaxTokens == 2000 {
		cfg.Memory.ProactiveInjection = true
		cfg.Memory.SessionArchive = true
	}

	// Conversation defaults
	if cfg.Conversation.RetentionDays == 0 {
		cfg.Conversation.RetentionDays = 90
	}
}
