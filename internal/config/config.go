// Package config handles parsing and validation of pylon configuration files.
// Spec Reference: Section 16 "config.yml Schema"
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the full pylon workspace configuration.
// Spec Reference: Section 16 "Full Schema"
type Config struct {
	Version      string                   `yaml:"version"`
	Runtime      RuntimeConfig            `yaml:"runtime"`
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
	Backend              string            `yaml:"backend"`
	MaxConcurrent        int               `yaml:"max_concurrent"`
	TaskTimeout          string            `yaml:"task_timeout"`
	MaxAttempts          int               `yaml:"max_attempts"`
	MaxTurns             int               `yaml:"max_turns"`
	PermissionMode       string            `yaml:"permission_mode"`
	AutoApproveTaskReview bool             `yaml:"auto_approve_task_review"`
	Env                  map[string]string `yaml:"env"`
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
	Host          string `yaml:"host"`
	Port          int    `yaml:"port"`
	AutoDashboard bool   `yaml:"auto_dashboard"`
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

// ParseTaskTimeout parses the TaskTimeout string into a time.Duration.
// Returns 30 minutes as fallback if parsing fails or the value is non-positive.
func (r RuntimeConfig) ParseTaskTimeout() time.Duration {
	d, err := time.ParseDuration(r.TaskTimeout)
	if err != nil || d <= 0 {
		return 30 * time.Minute
	}
	return d
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
	// Pre-initialize bool fields that should default to true.
	// go-yaml v3 only overwrites fields present in the YAML,
	// so pre-initialized values are preserved when not explicitly set.
	cfg := &Config{
		Git: GitConfig{
			AutoPush: true,
			Worktree: WorktreeConfig{
				Enabled:     true,
				AutoCleanup: true,
			},
		},
		Wiki: WikiConfig{
			AutoUpdate: true,
		},
		Memory: MemoryConfig{
			ProactiveInjection: true,
			SessionArchive:     true,
		},
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Validate required fields
	if cfg.Version == "" {
		return nil, fmt.Errorf("config validation error: 'version' is required")
	}

	// Apply defaults for non-bool fields (Spec Section 16 defaults)
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

	// Git defaults
	if cfg.Git.BranchPrefix == "" {
		cfg.Git.BranchPrefix = "task"
	}
	if cfg.Git.DefaultBase == "" {
		cfg.Git.DefaultBase = "main"
	}
	// Bool fields (AutoPush, Worktree.Enabled, etc.) are handled by
	// pre-initialization in ParseConfig. Only set non-bool defaults here.

	// Wiki defaults: set update_on triggers if auto_update is true but no triggers specified
	if cfg.Wiki.AutoUpdate && len(cfg.Wiki.UpdateOn) == 0 {
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
	// ProactiveInjection and SessionArchive default to true,
	// handled by pre-initialization in ParseConfig.

	// Conversation defaults
	if cfg.Conversation.RetentionDays == 0 {
		cfg.Conversation.RetentionDays = 90
	}
}
