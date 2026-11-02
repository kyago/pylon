package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
		{"runtime.provider", cfg.Runtime.Provider, "claude-code"},
		{"runtime.permission_mode", cfg.Runtime.PermissionMode, "acceptEdits"},
		{"git.branch_prefix", cfg.Git.BranchPrefix, "task"},
		{"git.default_base", cfg.Git.DefaultBase, "main"},
		{"git.pr.auto_pr", cfg.Git.PR.AutoPR, true},
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

	// Check runtime env
	if v, ok := cfg.Runtime.Env["CLAUDE_CODE_EFFORT_LEVEL"]; !ok || v != "high" {
		t.Errorf("expected CLAUDE_CODE_EFFORT_LEVEL=high, got %v", cfg.Runtime.Env)
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
	if cfg.Runtime.Provider != "claude-code" {
		t.Errorf("runtime.provider: got %q, expected %q", cfg.Runtime.Provider, "claude-code")
	}

	// Verify defaults were applied
	defaults := []struct {
		name     string
		got      interface{}
		expected interface{}
	}{
		{"git.branch_prefix (default)", cfg.Git.BranchPrefix, "task"},
		{"git.default_base (default)", cfg.Git.DefaultBase, "main"},
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
	if cfg.Runtime.Provider != "auto" {
		t.Errorf("expected default provider auto, got %q", cfg.Runtime.Provider)
	}
}

func TestParseConfig_RuntimeKnobsHaveNoDefaults(t *testing.T) {
	cfg, err := ParseConfig([]byte(`version: "0.1"`))
	if err != nil {
		t.Fatal(err)
	}
	// env 기본 주입은 세션·에이전트별 effort 설정을 덮어쓰므로 비워 둔다.
	if cfg.Runtime.Env != nil {
		t.Errorf("runtime.env must stay nil by default, got %v", cfg.Runtime.Env)
	}
	if cfg.Runtime.Effort != "" || cfg.Runtime.Model != "" {
		t.Errorf("effort/model must have no default, got effort=%q model=%q", cfg.Runtime.Effort, cfg.Runtime.Model)
	}

	cfg, err = ParseConfig([]byte("version: \"0.1\"\nruntime:\n  effort: xhigh\n  model: opus\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Runtime.Effort != "xhigh" || cfg.Runtime.Model != "opus" {
		t.Errorf("effort/model = %q/%q, want xhigh/opus", cfg.Runtime.Effort, cfg.Runtime.Model)
	}
}

func TestParseConfigProviderNeutralRuntime(t *testing.T) {
	data := []byte(`version: "0.2"
runtime:
  provider: provider-b
providers:
  provider-b:
    enabled: auto
    command: provider-b
`)

	cfg, err := ParseConfig(data)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Runtime.Provider != "provider-b" {
		t.Fatalf("runtime = %+v", cfg.Runtime)
	}
	providerConfig, ok := cfg.Providers["provider-b"]
	if !ok || providerConfig.Enabled != "auto" || providerConfig.Command != "provider-b" {
		t.Fatalf("provider config = %+v, exists=%v", providerConfig, ok)
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
  provider: openai
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
		{"runtime.provider", cfg.Runtime.Provider, "openai"},
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
	// Config with a git.pr section but missing auto_pr: a depth-2 field.
	content := []byte(`version: "0.1"
git:
  branch_prefix: custom
  default_base: develop
  pr: {}
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

	// Should detect git.pr.auto_pr as missing
	addedSet := make(map[string]bool)
	for _, a := range added {
		addedSet[a] = true
	}

	mustExist := []string{"git.pr.auto_pr"}
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

	// Verify the file was rewritten with missing fields
	reloaded, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig after sync failed: %v", err)
	}
	if reloaded.Git.BranchPrefix != "custom" {
		t.Errorf("expected branch_prefix=custom in rewritten file, got %q", reloaded.Git.BranchPrefix)
	}
	if raw, _ := os.ReadFile(cfgPath); !strings.Contains(string(raw), "auto_pr: false") {
		t.Errorf("expected auto_pr default in rewritten file:\n%s", raw)
	}
}

func TestSyncConfigDefaults_EntireSectionMissing(t *testing.T) {
	// Config with only version and runtime — skills section entirely missing.
	// Should be appended (not rewrite) and detected.
	content := []byte(`version: "0.1"
runtime:
  provider: claude-code
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

func TestSetRuntimeProvider(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	content := `version: "0.2"
# runtime section
runtime:
  provider: auto
  permission_mode: bypassPermissions
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if err := SetRuntimeProvider(path, "codex"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "provider: codex") || !strings.Contains(string(data), "# runtime section") {
		t.Fatalf("provider not pinned or comment lost:\n%s", data)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if providerName := cfg.Runtime.EffectiveProvider(); providerName != "codex" {
		t.Fatalf("effective provider = %q", providerName)
	}
	if cfg.Runtime.PermissionMode != "bypassPermissions" {
		t.Fatalf("sibling keys must survive: %+v", cfg.Runtime)
	}
}

func TestSetRuntimeProvider_CreatesMissingSectionAndKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(path, []byte("version: \"0.2\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := SetRuntimeProvider(path, "claude-code"); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if providerName := cfg.Runtime.EffectiveProvider(); providerName != "claude-code" {
		t.Fatalf("effective provider = %q", providerName)
	}
}

func TestSetRuntimeProvider_RejectsMultiDocument(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	content := "version: \"0.2\"\n---\nsecond: doc\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if err := SetRuntimeProvider(path, "codex"); err == nil {
		t.Fatal("multi-document config must be rejected")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != content {
		t.Fatalf("file must be untouched:\n%s", data)
	}
}

func TestSetRuntimeProvider_EmptyRuntimeSection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(path, []byte("version: \"0.2\"\nruntime:\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := SetRuntimeProvider(path, "codex"); err != nil {
		t.Fatalf("empty runtime section must be treated as an empty mapping: %v", err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if providerName := cfg.Runtime.EffectiveProvider(); providerName != "codex" {
		t.Fatalf("effective provider = %q", providerName)
	}
}
