// Package config handles parsing and validation of pylon configuration files.
// Spec Reference: Section 16 "config.yml Schema"
package config

import (
	"fmt"
	"os"
	"strings"
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

// SyncConfigDefaults reads config.yml, detects missing fields, and appends them.
// Only writes to disk if there are actually missing fields, preserving existing content.
// Returns the updated Config and a list of field paths that were added.
func SyncConfigDefaults(path string) (*Config, []string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read config file: %w", err)
	}

	cfg, err := ParseConfig(raw)
	if err != nil {
		return nil, nil, err
	}

	// Parse raw into nested map to detect existing keys
	var rawMap map[string]any
	if unmarshalErr := yaml.Unmarshal(raw, &rawMap); unmarshalErr != nil {
		rawMap = make(map[string]any)
	}

	// Build defaults as map for comparison
	defaultBytes, _ := yaml.Marshal(cfg)
	var defaultMap map[string]any
	_ = yaml.Unmarshal(defaultBytes, &defaultMap)

	// Find missing fields and build append content
	var added []string
	var appendBuf []byte

	for key, defaultVal := range defaultMap {
		rawVal, exists := rawMap[key]
		if !exists {
			// Entire section missing — append it
			section, _ := yaml.Marshal(map[string]any{key: defaultVal})
			appendBuf = append(appendBuf, '\n')
			appendBuf = append(appendBuf, section...)
			added = append(added, key)
			continue
		}

		// Section exists — check sub-keys
		defaultSub, ok := defaultVal.(map[string]any)
		if !ok {
			continue
		}
		rawSub, _ := rawVal.(map[string]any)
		if rawSub == nil {
			rawSub = make(map[string]any)
		}

		var missingKeys []string
		for subKey := range defaultSub {
			if _, subExists := rawSub[subKey]; !subExists {
				missingKeys = append(missingKeys, subKey)
				added = append(added, key+"."+subKey)
			}
		}

		// If sub-keys are missing, rewrite just this section with merged values
		if len(missingKeys) > 0 {
			merged := make(map[string]any)
			for k, v := range rawSub {
				merged[k] = v
			}
			for k, v := range defaultSub {
				if _, exists := rawSub[k]; !exists {
					merged[k] = v
				}
			}
			// Replace the section in-place by rewriting the full file with the merged map
			rawMap[key] = merged
		}
	}

	if len(added) == 0 {
		return cfg, nil, nil
	}

	// If we had sub-key merges (rawMap was modified), rewrite the full file
	// Otherwise just append missing sections
	hasSubKeyMerge := false
	for _, a := range added {
		if strings.Contains(a, ".") {
			hasSubKeyMerge = true
			break
		}
	}

	if hasSubKeyMerge {
		data, err := yaml.Marshal(rawMap)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to marshal config: %w", err)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			return nil, nil, fmt.Errorf("failed to write config: %w", err)
		}
	} else if len(appendBuf) > 0 {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to open config for append: %w", err)
		}
		defer f.Close()
		if _, err := f.Write(appendBuf); err != nil {
			return nil, nil, fmt.Errorf("failed to append config: %w", err)
		}
	}

	return cfg, added, nil
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
		Dashboard: DashboardConfig{
			AutoDashboard: true,
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
