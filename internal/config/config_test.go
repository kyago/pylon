package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseConfig_FullConfig(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "config", "full_config.yml"))
	if err != nil {
		t.Fatalf("failed to read test fixture: %v", err)
	}

	cfg, err := ParseConfig(data)
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}

	tests := []struct {
		name     string
		got      interface{}
		expected interface{}
	}{
		{"version", cfg.Version, "0.1"},
		{"runtime.backend", cfg.Runtime.Backend, "claude-code"},
		{"runtime.max_concurrent", cfg.Runtime.MaxConcurrent, 5},
		{"runtime.task_timeout", cfg.Runtime.TaskTimeout, "30m"},
		{"runtime.max_attempts", cfg.Runtime.MaxAttempts, 2},
		{"runtime.max_turns", cfg.Runtime.MaxTurns, 50},
		{"runtime.permission_mode", cfg.Runtime.PermissionMode, "acceptEdits"},
		{"git.branch_prefix", cfg.Git.BranchPrefix, "task"},
		{"git.default_base", cfg.Git.DefaultBase, "main"},
		{"git.auto_push", cfg.Git.AutoPush, true},
		{"git.worktree.enabled", cfg.Git.Worktree.Enabled, true},
		{"git.worktree.auto_cleanup", cfg.Git.Worktree.AutoCleanup, true},
		{"git.pr.draft", cfg.Git.PR.Draft, false},
		{"wiki.auto_update", cfg.Wiki.AutoUpdate, true},
		{"dashboard.host", cfg.Dashboard.Host, "localhost"},
		{"dashboard.port", cfg.Dashboard.Port, 7777},
		{"memory.compaction_threshold", cfg.Memory.CompactionThreshold, 0.7},
		{"memory.proactive_injection", cfg.Memory.ProactiveInjection, true},
		{"memory.proactive_max_tokens", cfg.Memory.ProactiveMaxTokens, 2000},
		{"memory.session_archive", cfg.Memory.SessionArchive, true},
		{"memory.retention_days", cfg.Memory.RetentionDays, 0},
		{"conversation.retention_days", cfg.Conversation.RetentionDays, 90},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("got %v, expected %v", tt.got, tt.expected)
			}
		})
	}

	// Check reviewers list
	if len(cfg.Git.PR.Reviewers) != 1 || cfg.Git.PR.Reviewers[0] != "keiyjay" {
		t.Errorf("expected reviewers [keiyjay], got %v", cfg.Git.PR.Reviewers)
	}

	// Check projects
	if len(cfg.Projects) != 2 {
		t.Errorf("expected 2 projects, got %d", len(cfg.Projects))
	}
	if p, ok := cfg.Projects["project-api"]; !ok || p.Stack != "go" {
		t.Errorf("expected project-api with stack=go, got %v", cfg.Projects["project-api"])
	}

	// Check runtime env
	if v, ok := cfg.Runtime.Env["CLAUDE_CODE_EFFORT_LEVEL"]; !ok || v != "high" {
		t.Errorf("expected CLAUDE_CODE_EFFORT_LEVEL=high, got %v", cfg.Runtime.Env)
	}

	// Check wiki update_on
	if len(cfg.Wiki.UpdateOn) != 2 {
		t.Errorf("expected 2 update_on triggers, got %d", len(cfg.Wiki.UpdateOn))
	}
}

func TestParseConfig_MinimalConfig(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "config", "minimal_config.yml"))
	if err != nil {
		t.Fatalf("failed to read test fixture: %v", err)
	}

	cfg, err := ParseConfig(data)
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}

	// Verify explicit values
	if cfg.Version != "0.1" {
		t.Errorf("version: got %q, expected %q", cfg.Version, "0.1")
	}
	if cfg.Runtime.Backend != "claude-code" {
		t.Errorf("runtime.backend: got %q, expected %q", cfg.Runtime.Backend, "claude-code")
	}
	if cfg.Runtime.MaxConcurrent != 5 {
		t.Errorf("runtime.max_concurrent: got %d, expected %d", cfg.Runtime.MaxConcurrent, 5)
	}

	// Verify defaults were applied
	defaults := []struct {
		name     string
		got      interface{}
		expected interface{}
	}{
		{"runtime.task_timeout (default)", cfg.Runtime.TaskTimeout, "30m"},
		{"runtime.max_attempts (default)", cfg.Runtime.MaxAttempts, 2},
		{"git.branch_prefix (default)", cfg.Git.BranchPrefix, "task"},
		{"git.default_base (default)", cfg.Git.DefaultBase, "main"},
		{"git.auto_push (default)", cfg.Git.AutoPush, true},
		{"git.worktree.enabled (default)", cfg.Git.Worktree.Enabled, true},
		{"git.worktree.auto_cleanup (default)", cfg.Git.Worktree.AutoCleanup, true},
		{"wiki.auto_update (default)", cfg.Wiki.AutoUpdate, true},
		{"memory.proactive_injection (default)", cfg.Memory.ProactiveInjection, true},
		{"memory.session_archive (default)", cfg.Memory.SessionArchive, true},
		{"dashboard.host (default)", cfg.Dashboard.Host, "localhost"},
		{"dashboard.port (default)", cfg.Dashboard.Port, 7777},
		{"memory.compaction_threshold (default)", cfg.Memory.CompactionThreshold, 0.7},
		{"memory.proactive_max_tokens (default)", cfg.Memory.ProactiveMaxTokens, 2000},
		{"conversation.retention_days (default)", cfg.Conversation.RetentionDays, 90},
	}

	for _, tt := range defaults {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("got %v, expected %v", tt.got, tt.expected)
			}
		})
	}
}

func TestParseConfig_MissingVersion(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "config", "missing_version.yml"))
	if err != nil {
		t.Fatalf("failed to read test fixture: %v", err)
	}

	_, err = ParseConfig(data)
	if err == nil {
		t.Fatal("expected error for missing version, got nil")
	}
}

func TestParseConfig_InvalidYAML(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "config", "invalid_yaml.yml"))
	if err != nil {
		t.Fatalf("failed to read test fixture: %v", err)
	}

	_, err = ParseConfig(data)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestParseConfig_EmptyInput(t *testing.T) {
	_, err := ParseConfig([]byte(""))
	if err == nil {
		t.Fatal("expected error for empty input, got nil")
	}
}

func TestParseConfig_VersionOnly(t *testing.T) {
	data := []byte(`version: "0.1"`)
	cfg, err := ParseConfig(data)
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}

	// All defaults should be applied
	if cfg.Runtime.Backend != "claude-code" {
		t.Errorf("expected default backend claude-code, got %q", cfg.Runtime.Backend)
	}
	if cfg.Runtime.MaxConcurrent != 5 {
		t.Errorf("expected default max_concurrent 5, got %d", cfg.Runtime.MaxConcurrent)
	}
}

func TestParseTaskTimeout(t *testing.T) {
	tests := []struct {
		name     string
		timeout  string
		expected time.Duration
	}{
		{"기본 30분", "30m", 30 * time.Minute},
		{"1시간", "1h", time.Hour},
		{"10초", "10s", 10 * time.Second},
		{"2시간30분", "2h30m", 2*time.Hour + 30*time.Minute},
		{"0초 fallback", "0s", 30 * time.Minute},
		{"음수 fallback", "-5m", 30 * time.Minute},
		{"잘못된 형식 fallback", "invalid", 30 * time.Minute},
		{"빈 문자열 fallback", "", 30 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc := RuntimeConfig{TaskTimeout: tt.timeout}
			got := rc.ParseTaskTimeout()
			if got != tt.expected {
				t.Errorf("ParseTaskTimeout(%q) = %v, want %v", tt.timeout, got, tt.expected)
			}
		})
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/config.yml")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}

func TestParseConfig_CustomValues(t *testing.T) {
	data := []byte(`
version: "0.2"
runtime:
  backend: openai
  max_concurrent: 10
  task_timeout: 1h
  max_attempts: 3
  max_turns: 100
  permission_mode: bypassPermissions
git:
  branch_prefix: feature
  default_base: develop
dashboard:
  host: 0.0.0.0
  port: 8080
memory:
  compaction_threshold: 0.8
  proactive_max_tokens: 4000
conversation:
  retention_days: 30
`)

	cfg, err := ParseConfig(data)
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}

	tests := []struct {
		name     string
		got      interface{}
		expected interface{}
	}{
		{"version", cfg.Version, "0.2"},
		{"runtime.backend", cfg.Runtime.Backend, "openai"},
		{"runtime.max_concurrent", cfg.Runtime.MaxConcurrent, 10},
		{"runtime.task_timeout", cfg.Runtime.TaskTimeout, "1h"},
		{"runtime.max_attempts", cfg.Runtime.MaxAttempts, 3},
		{"runtime.max_turns", cfg.Runtime.MaxTurns, 100},
		{"runtime.permission_mode", cfg.Runtime.PermissionMode, "bypassPermissions"},
		{"git.branch_prefix", cfg.Git.BranchPrefix, "feature"},
		{"git.default_base", cfg.Git.DefaultBase, "develop"},
		{"dashboard.host", cfg.Dashboard.Host, "0.0.0.0"},
		{"dashboard.port", cfg.Dashboard.Port, 8080},
		{"memory.compaction_threshold", cfg.Memory.CompactionThreshold, 0.8},
		{"memory.proactive_max_tokens", cfg.Memory.ProactiveMaxTokens, 4000},
		{"conversation.retention_days", cfg.Conversation.RetentionDays, 30},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("got %v, expected %v", tt.got, tt.expected)
			}
		})
	}
}
