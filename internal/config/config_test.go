package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
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
		{"memory.proactive_injection", cfg.Memory.ProactiveInjection, true},
		{"memory.proactive_max_tokens", cfg.Memory.ProactiveMaxTokens, 2000},
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
		{"memory.proactive_max_tokens (default)", cfg.Memory.ProactiveMaxTokens, 2000},
	}

	for _, tt := range defaults {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("got %v, expected %v", tt.got, tt.expected)
			}
		})
	}
}

func TestParseConfig_IgnoresRemovedFields(t *testing.T) {
	yml := `
version: "1"
memory:
  session_archive: true
  retention_days:
    learning: 30
conversation:
  retention_days: 90
`
	cfg, err := ParseConfig([]byte(yml))
	if err != nil {
		t.Fatalf("removed fields must be ignored, got error: %v", err)
	}
	if cfg.Memory.ProactiveMaxTokens != 2000 {
		t.Fatalf("defaults must still apply: %v", cfg.Memory.ProactiveMaxTokens)
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
	if cfg.Runtime.Provider != "auto" || cfg.Runtime.Backend != "" {
		t.Errorf("expected default provider auto without backend alias, got provider=%q backend=%q", cfg.Runtime.Provider, cfg.Runtime.Backend)
	}
	if cfg.Runtime.MaxConcurrent != 5 {
		t.Errorf("expected default max_concurrent 5, got %d", cfg.Runtime.MaxConcurrent)
	}
}

func TestRuntimeConfigEffectiveProviderCompatibility(t *testing.T) {
	tests := []struct {
		name       string
		runtime    RuntimeConfig
		provider   string
		deprecated bool
	}{
		{name: "provider wins", runtime: RuntimeConfig{Provider: "provider-b", Backend: "claude-code"}, provider: "provider-b"},
		{name: "backend alias", runtime: RuntimeConfig{Backend: "claude-code"}, provider: "claude-code", deprecated: true},
		{name: "implicit auto", runtime: RuntimeConfig{}, provider: "auto"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, deprecated := tt.runtime.EffectiveProvider()
			if provider != tt.provider || deprecated != tt.deprecated {
				t.Fatalf("EffectiveProvider() = %q, %v; want %q, %v", provider, deprecated, tt.provider, tt.deprecated)
			}
		})
	}
}

func TestParseConfigProviderNeutralRuntime(t *testing.T) {
	data := []byte(`version: "0.2"
runtime:
  provider: provider-b
  execution_mode: managed-adapter
providers:
  provider-b:
    enabled: auto
    command: provider-b
routing:
  fallback: serial
  require_resume_for_background: false
`)

	cfg, err := ParseConfig(data)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Runtime.Provider != "provider-b" || cfg.Runtime.ExecutionMode != "managed-adapter" {
		t.Fatalf("runtime = %+v", cfg.Runtime)
	}
	providerConfig, ok := cfg.Providers["provider-b"]
	if !ok || providerConfig.Enabled != "auto" || providerConfig.Command != "provider-b" {
		t.Fatalf("provider config = %+v, exists=%v", providerConfig, ok)
	}
	if cfg.Routing.Fallback != "serial" || cfg.Routing.RequireResumeForBackground {
		t.Fatalf("routing = %+v", cfg.Routing)
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

func TestParseConfig_WorkflowDefaults(t *testing.T) {
	data := []byte(`version: "0.1"`)
	cfg, err := ParseConfig(data)
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}

	if cfg.Workflow.DefaultWorkflow != "feature" {
		t.Errorf("expected default workflow 'feature', got %q", cfg.Workflow.DefaultWorkflow)
	}
}

func TestParseConfig_WorkflowCustomValues(t *testing.T) {
	data := []byte(`
version: "0.1"
workflow:
  default_workflow: bugfix
  template_dir: .pylon/workflows
`)
	cfg, err := ParseConfig(data)
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}

	if cfg.Workflow.DefaultWorkflow != "bugfix" {
		t.Errorf("expected workflow 'bugfix', got %q", cfg.Workflow.DefaultWorkflow)
	}
	if cfg.Workflow.TemplateDir != ".pylon/workflows" {
		t.Errorf("expected template_dir '.pylon/workflows', got %q", cfg.Workflow.TemplateDir)
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
memory:
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
		{"memory.proactive_max_tokens", cfg.Memory.ProactiveMaxTokens, 4000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("got %v, expected %v", tt.got, tt.expected)
			}
		})
	}
}

func TestSyncConfigDefaults_DeepNestedMissing(t *testing.T) {
	// Config with git.worktree section but missing auto_cleanup,
	// and git.pr section but missing draft.
	// These are depth-2 fields that the old code could not detect.
	content := []byte(`version: "0.1"
git:
  branch_prefix: custom
  default_base: develop
  worktree:
    enabled: true
  pr:
    auto_pr: true
    reviewers:
      - alice
`)

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yml")
	if err := os.WriteFile(cfgPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, added, err := SyncConfigDefaults(cfgPath)
	if err != nil {
		t.Fatalf("SyncConfigDefaults failed: %v", err)
	}

	// Should detect git.worktree.auto_cleanup and git.pr.draft as missing
	addedSet := make(map[string]bool)
	for _, a := range added {
		addedSet[a] = true
	}

	mustExist := []string{"git.worktree.auto_cleanup", "git.pr.draft"}
	for _, field := range mustExist {
		if !addedSet[field] {
			t.Errorf("expected %q in added list, got: %v", field, added)
		}
	}

	// Existing values must be preserved
	if cfg.Git.BranchPrefix != "custom" {
		t.Errorf("expected branch_prefix=custom, got %q", cfg.Git.BranchPrefix)
	}
	if cfg.Git.DefaultBase != "develop" {
		t.Errorf("expected default_base=develop, got %q", cfg.Git.DefaultBase)
	}
	if !cfg.Git.Worktree.Enabled {
		t.Error("expected worktree.enabled=true to be preserved")
	}
	if !cfg.Git.PR.AutoPR {
		t.Error("expected pr.auto_pr=true to be preserved")
	}

	// Verify the file was rewritten with missing fields
	reloaded, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig after sync failed: %v", err)
	}
	if !reloaded.Git.Worktree.AutoCleanup {
		t.Error("expected worktree.auto_cleanup=true in rewritten file")
	}
}

func TestSyncConfigDefaults_EntireSectionMissing(t *testing.T) {
	// Config with only version and runtime — skills section entirely missing.
	// Should be appended (not rewrite) and detected.
	content := []byte(`version: "0.1"
runtime:
  backend: claude-code
  max_concurrent: 5
  task_timeout: 30m
  max_attempts: 2
  max_turns: 50
  permission_mode: acceptEdits
  env:
    CLAUDE_AUTOCOMPACT_PCT_OVERRIDE: "80"
    CLAUDE_CODE_EFFORT_LEVEL: high
git:
  branch_prefix: task
  default_base: main
  auto_push: true
  worktree:
    enabled: true
    auto_cleanup: true
  pr:
    auto_pr: false
    draft: false
wiki:
  auto_update: true
  update_on:
    - task_complete
    - pr_merged
memory:
  proactive_injection: true
  proactive_max_tokens: 2000
  session_archive: true
conversation:
  retention_days: 90
workflow:
  default_workflow: feature
`)

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yml")
	if err := os.WriteFile(cfgPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	_, added, err := SyncConfigDefaults(cfgPath)
	if err != nil {
		t.Fatalf("SyncConfigDefaults failed: %v", err)
	}

	// skills section is entirely missing — should be detected
	addedSet := make(map[string]bool)
	for _, a := range added {
		addedSet[a] = true
	}

	if !addedSet["skills"] {
		t.Errorf("expected 'skills' in added list, got: %v", added)
	}
}

func TestSyncConfigDefaultsDoesNotRewriteBackendAliasToProvider(t *testing.T) {
	content := []byte(`version: "0.1"
runtime:
  backend: claude-code
  max_concurrent: 5
  max_turns: 50
  permission_mode: acceptEdits
`)

	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := SyncConfigDefaults(path); err != nil {
		t.Fatal(err)
	}
	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := yaml.Unmarshal(updated, &raw); err != nil {
		t.Fatal(err)
	}
	runtime := raw["runtime"].(map[string]any)
	if _, exists := runtime["provider"]; exists {
		t.Fatalf("deprecated backend alias was rewritten to provider: %s", updated)
	}
}

func TestSyncConfigDefaults_NoChangesOnSecondSync(t *testing.T) {
	// Minimal config → first sync adds defaults → second sync should find no changes.
	content := []byte(`version: "0.1"`)

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yml")
	if err := os.WriteFile(cfgPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	// First sync: adds all missing defaults
	_, firstAdded, err := SyncConfigDefaults(cfgPath)
	if err != nil {
		t.Fatalf("first SyncConfigDefaults failed: %v", err)
	}
	if len(firstAdded) == 0 {
		t.Fatal("expected first sync to add fields for minimal config")
	}

	// Second sync: should find nothing missing
	_, secondAdded, err := SyncConfigDefaults(cfgPath)
	if err != nil {
		t.Fatalf("second SyncConfigDefaults failed: %v", err)
	}
	if len(secondAdded) != 0 {
		t.Errorf("expected no changes on second sync, got added: %v", secondAdded)
	}
}

func TestFindMissingFields_Recursive(t *testing.T) {
	defaults := map[string]any{
		"a": "val_a",
		"b": map[string]any{
			"b1": "val_b1",
			"b2": map[string]any{
				"b2a": "val_b2a",
				"b2b": "val_b2b",
			},
		},
	}

	dst := map[string]any{
		"a": "custom_a",
		"b": map[string]any{
			"b1": "custom_b1",
			"b2": map[string]any{
				"b2a": "custom_b2a",
				// b2b is missing
			},
		},
	}

	added := findMissingFields(dst, defaults, "root")

	if len(added) != 1 {
		t.Fatalf("expected 1 added field, got %d: %v", len(added), added)
	}
	if added[0] != "root.b.b2.b2b" {
		t.Errorf("expected root.b.b2.b2b, got %q", added[0])
	}

	// Verify the value was merged
	b := dst["b"].(map[string]any)
	b2 := b["b2"].(map[string]any)
	if b2["b2b"] != "val_b2b" {
		t.Errorf("expected b2b=val_b2b, got %v", b2["b2b"])
	}
	// Existing value must be preserved
	if b2["b2a"] != "custom_b2a" {
		t.Errorf("expected b2a=custom_b2a preserved, got %v", b2["b2a"])
	}
}

func TestFindMissingFields_TypeMismatchGuard(t *testing.T) {
	// When user writes a scalar where a map is expected (e.g., worktree: true),
	// findMissingFields should still fill in default sub-keys.
	defaults := map[string]any{
		"nested": map[string]any{
			"child_a": "val_a",
			"child_b": "val_b",
		},
	}

	dst := map[string]any{
		"nested": "scalar_value", // wrong type: scalar instead of map
	}

	added := findMissingFields(dst, defaults, "root")

	if len(added) != 2 {
		t.Fatalf("expected 2 added fields, got %d: %v", len(added), added)
	}

	// dst["nested"] should now be a map with defaults
	nested, ok := dst["nested"].(map[string]any)
	if !ok {
		t.Fatalf("expected dst[nested] to be map, got %T", dst["nested"])
	}
	if nested["child_a"] != "val_a" || nested["child_b"] != "val_b" {
		t.Errorf("expected default sub-keys, got %v", nested)
	}
}

func TestMemoryRetentionDaysDefault(t *testing.T) {
	cfg, err := ParseConfig([]byte("version: \"0.1\"\n"))
	if err != nil {
		t.Fatalf("ParseConfig 실패: %v", err)
	}
	if got := cfg.Memory.RetentionDays["learning"]; got != 30 {
		t.Errorf("retention_days.learning 기본값 = %d, want 30", got)
	}
}

func TestMemoryRetentionDaysOverride(t *testing.T) {
	yml := "version: \"0.1\"\nmemory:\n  retention_days:\n    learning: 7\n    note: 14\n"
	cfg, err := ParseConfig([]byte(yml))
	if err != nil {
		t.Fatalf("ParseConfig 실패: %v", err)
	}
	if cfg.Memory.RetentionDays["learning"] != 7 || cfg.Memory.RetentionDays["note"] != 14 {
		t.Errorf("명시값이 적용되어야 한다: %v", cfg.Memory.RetentionDays)
	}
}

// 빈 맵을 명시하면 "모든 카테고리 영구 보존" opt-out이다 (기본값 미적용).
func TestMemoryRetentionDaysExplicitEmpty(t *testing.T) {
	yml := "version: \"0.1\"\nmemory:\n  retention_days: {}\n"
	cfg, err := ParseConfig([]byte(yml))
	if err != nil {
		t.Fatalf("ParseConfig 실패: %v", err)
	}
	if len(cfg.Memory.RetentionDays) != 0 {
		t.Errorf("빈 맵 명시는 보존 정책 해제여야 한다: %v", cfg.Memory.RetentionDays)
	}
}

func TestMemoryRetentionDaysLegacyScalar(t *testing.T) {
	cfg, err := ParseConfig([]byte("version: \"0.1\"\nmemory:\n  retention_days: 0\n"))
	if err != nil {
		t.Fatalf("레거시 스칼라 retention_days는 파싱 에러 없이 로드되어야 한다: %v", err)
	}
	if len(cfg.Memory.RetentionDays) != 0 {
		t.Errorf("레거시 스칼라는 정책 없음(빈)이어야 한다: %v", cfg.Memory.RetentionDays)
	}
}

func TestMigrateRuntimeBackend_RenamesKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	content := `version: "0.2"
# runtime settings
runtime:
  backend: claude-code # my provider
  max_concurrent: 3
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	migrated, err := MigrateRuntimeBackend(path)
	if err != nil || !migrated {
		t.Fatalf("migrated=%v err=%v", migrated, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "backend:") {
		t.Fatalf("backend key must be gone:\n%s", text)
	}
	if !strings.Contains(text, "provider: claude-code") {
		t.Fatalf("provider key missing:\n%s", text)
	}
	if !strings.Contains(text, "# runtime settings") || !strings.Contains(text, "# my provider") {
		t.Fatalf("comments must be preserved:\n%s", text)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	providerName, deprecated := cfg.Runtime.EffectiveProvider()
	if providerName != "claude-code" || deprecated {
		t.Fatalf("effective provider = %q deprecated=%v", providerName, deprecated)
	}
	if cfg.Runtime.MaxConcurrent != 3 {
		t.Fatalf("sibling keys must survive: %+v", cfg.Runtime)
	}
}

func TestMigrateRuntimeBackend_DropsBackendWhenProviderExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	content := "version: \"0.2\"\nruntime:\n  provider: codex\n  backend: claude-code\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	migrated, err := MigrateRuntimeBackend(path)
	if err != nil || !migrated {
		t.Fatalf("migrated=%v err=%v", migrated, err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	// provider가 우선이었으므로 backend 제거는 의미 변화가 없어야 한다.
	if providerName, deprecated := cfg.Runtime.EffectiveProvider(); providerName != "codex" || deprecated {
		t.Fatalf("effective provider = %q deprecated=%v", providerName, deprecated)
	}
	if cfg.Runtime.Backend != "" {
		t.Fatalf("backend must be removed: %+v", cfg.Runtime)
	}
}

func TestMigrateRuntimeBackend_NoopWithoutBackend(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	content := "version: \"0.2\"\nruntime:\n  provider: claude-code\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	migrated, err := MigrateRuntimeBackend(path)
	if err != nil || migrated {
		t.Fatalf("expected noop: migrated=%v err=%v", migrated, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != content {
		t.Fatalf("file must be untouched on noop:\n%s", data)
	}
}

func TestMigrateRuntimeBackend_BailsOnMultiDocument(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	content := "version: \"0.2\"\nruntime:\n  backend: codex\n---\nsecond: doc\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	migrated, err := MigrateRuntimeBackend(path)
	if err != nil || migrated {
		t.Fatalf("multi-document file must not be migrated: migrated=%v err=%v", migrated, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != content {
		t.Fatalf("multi-document file must be untouched:\n%s", data)
	}
}

func TestMigrateRuntimeBackend_EmptyProviderDoesNotLoseBackend(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	content := "version: \"0.2\"\nruntime:\n  provider: \"\"\n  backend: codex\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	migrated, err := MigrateRuntimeBackend(path)
	if err != nil || !migrated {
		t.Fatalf("migrated=%v err=%v", migrated, err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	// 기존 유효값(backend=codex)이 보존되어야 한다 — auto로 바뀌면 의미 변경.
	if providerName, deprecated := cfg.Runtime.EffectiveProvider(); providerName != "codex" || deprecated {
		t.Fatalf("effective provider = %q deprecated=%v", providerName, deprecated)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), "provider:") != 1 || strings.Contains(string(data), "backend:") {
		t.Fatalf("expected single provider key and no backend:\n%s", data)
	}
}

func TestMigrateRuntimeBackend_NoopWhenConfigMissing(t *testing.T) {
	migrated, err := MigrateRuntimeBackend(filepath.Join(t.TempDir(), "config.yml"))
	if err != nil || migrated {
		t.Fatalf("missing config must be a silent noop: migrated=%v err=%v", migrated, err)
	}
}
