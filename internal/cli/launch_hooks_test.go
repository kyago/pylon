package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kyago/pylon/internal/config"
)

func TestGenerateSettingsHooks(t *testing.T) {
	tmpDir := t.TempDir()

	if err := generateSettingsHooks(tmpDir); err != nil {
		t.Fatalf("generateSettingsHooks() error = %v", err)
	}

	settingsPath := filepath.Join(tmpDir, "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read settings.json: %v", err)
	}

	// Parse and verify structure
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("failed to parse settings.json: %v", err)
	}

	hooks, ok := settings["hooks"]
	if !ok {
		t.Fatal("settings.json should have a 'hooks' key")
	}

	hooksMap, ok := hooks.(map[string]any)
	if !ok {
		t.Fatal("hooks should be a map")
	}

	// Stop hook은 더 이상 설치하지 않는다 — 메모리는 SessionStart에서 주입한다.
	if _, ok := hooksMap["Stop"]; ok {
		t.Fatal("Stop hook should not be installed")
	}

	startHooks, ok := hooksMap["SessionStart"].([]any)
	if !ok || len(startHooks) != 1 {
		t.Fatalf("SessionStart should have exactly 1 group, got %v", hooksMap["SessionStart"])
	}
	startGroup, ok := startHooks[0].(map[string]any)
	if !ok {
		t.Fatal("SessionStart hook group should be a map")
	}
	if m, _ := startGroup["matcher"].(string); m != "startup|resume|clear|compact" {
		t.Errorf("SessionStart matcher = %q, want \"startup|resume|clear|compact\"", m)
	}
	inner, ok := startGroup["hooks"].([]any)
	if !ok || len(inner) != 1 {
		t.Fatal("SessionStart group should have exactly 1 hook")
	}
	startCmd, _ := inner[0].(map[string]any)
	if startCmd["type"] != "command" || startCmd["command"] != "pylon mem inject" {
		t.Errorf("SessionStart hook = %v, want command 'pylon mem inject'", startCmd)
	}

	// PostToolUse hook must NOT be installed by default: it stored raw edit
	// diffs as memory and polluted BM25 search (issue #76).
	if _, ok := hooksMap["PostToolUse"]; ok {
		t.Fatal("PostToolUse hook should not be installed by default (issue #76)")
	}

	// Verify no description field (not in Claude Code spec)
	if _, hasDesc := startGroup["description"]; hasDesc {
		t.Error("hook groups should not have a 'description' field")
	}
}

func TestGenerateSettingsHooks_ValidJSON(t *testing.T) {
	tmpDir := t.TempDir()

	if err := generateSettingsHooks(tmpDir); err != nil {
		t.Fatalf("generateSettingsHooks() error = %v", err)
	}

	settingsPath := filepath.Join(tmpDir, "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read settings.json: %v", err)
	}

	// Verify it's valid JSON
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("settings.json is not valid JSON: %v", err)
	}

	// Verify top-level "hooks" key exists
	if _, ok := raw["hooks"]; !ok {
		t.Error("settings.json should have a 'hooks' key")
	}
}

func TestGenerateSettingsHooks_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()

	// Generate twice
	if err := generateSettingsHooks(tmpDir); err != nil {
		t.Fatalf("first generateSettingsHooks() error = %v", err)
	}
	first, _ := os.ReadFile(filepath.Join(tmpDir, "settings.json"))

	if err := generateSettingsHooks(tmpDir); err != nil {
		t.Fatalf("second generateSettingsHooks() error = %v", err)
	}
	second, _ := os.ReadFile(filepath.Join(tmpDir, "settings.json"))

	if string(first) != string(second) {
		t.Error("generateSettingsHooks should be idempotent")
	}
}

func TestGenerateSettingsHooks_PreservesExisting(t *testing.T) {
	tmpDir := t.TempDir()

	// Write existing settings with user hooks (in Claude Code format)
	existingSettings := map[string]any{
		"theme": "dark",
		"hooks": map[string]any{
			"Stop": []any{
				map[string]any{
					"matcher": "",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "my-custom-hook --on-stop",
						},
					},
				},
				// 구버전이 설치한 pylon Stop hook — 병합 시 제거되어야 한다
				map[string]any{
					"matcher": "*",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "pylon sync-memory --from-session --agent claude",
						},
					},
				},
			},
			"PreToolUse": []any{
				map[string]any{
					"matcher": "",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "my-linter --check",
						},
					},
				},
			},
		},
	}
	data, _ := json.MarshalIndent(existingSettings, "", "  ")
	settingsPath := filepath.Join(tmpDir, "settings.json")
	if err := os.WriteFile(settingsPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	// Generate pylon hooks
	if err := generateSettingsHooks(tmpDir); err != nil {
		t.Fatalf("generateSettingsHooks() error = %v", err)
	}

	// Read result
	result, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal(result, &settings); err != nil {
		t.Fatal(err)
	}

	// Verify non-hook settings preserved
	if settings["theme"] != "dark" {
		t.Error("existing non-hook settings should be preserved")
	}

	// Verify user hooks preserved
	hooks := settings["hooks"].(map[string]any)

	// PreToolUse should be untouched
	preHooks, ok := hooks["PreToolUse"]
	if !ok {
		t.Fatal("user PreToolUse hook should be preserved")
	}
	preArr := preHooks.([]any)
	if len(preArr) != 1 {
		t.Errorf("PreToolUse should have 1 entry, got %d", len(preArr))
	}

	// Stop에는 사용자 hook만 남고 구버전 pylon Stop hook은 제거된다
	stopHooks := hooks["Stop"].([]any)
	if len(stopHooks) != 1 {
		t.Fatalf("Stop should keep only the user hook group, got %d", len(stopHooks))
	}
	userGroup := stopHooks[0].(map[string]any)
	userInnerHooks := userGroup["hooks"].([]any)
	userCmd := userInnerHooks[0].(map[string]any)
	if userCmd["command"] != "my-custom-hook --on-stop" {
		t.Error("user's custom Stop hook should be preserved")
	}

	// pylon hook은 SessionStart에 추가된다
	startHooks, ok := hooks["SessionStart"].([]any)
	if !ok || len(startHooks) != 1 {
		t.Fatalf("SessionStart should have the pylon hook group, got %v", hooks["SessionStart"])
	}
	pylonCmd := startHooks[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)
	if pylonCmd["command"] != "pylon mem inject" {
		t.Errorf("pylon SessionStart hook should be added, got %v", pylonCmd["command"])
	}
}

// 구버전 pylon Stop hook만 있던 settings.json은 병합 후 Stop 키가 사라지고
// SessionStart만 남는다.
func TestMergeHooks_DropsLegacyPylonStop(t *testing.T) {
	existing := map[string]any{
		"hooks": map[string]any{
			"Stop": []any{
				map[string]any{
					"matcher": "*",
					"hooks": []any{
						map[string]any{"type": "command", "command": "pylon sync-memory --from-session --agent claude"},
					},
				},
			},
		},
	}
	var pylonHooks map[string][]settingsHookEntry
	if err := json.Unmarshal(defaultHooksJSON, &pylonHooks); err != nil {
		t.Fatal(err)
	}

	merged := mergeHooks(existing, pylonHooks)
	if _, ok := merged["Stop"]; ok {
		t.Errorf("legacy pylon Stop hook should be removed, got %v", merged["Stop"])
	}
	if len(merged) != 1 || merged["SessionStart"] == nil {
		t.Errorf("only SessionStart should remain, got %v", merged)
	}
}

func TestGenerateSettingsHooks_CleansLegacyHooksJSON(t *testing.T) {
	tmpDir := t.TempDir()

	// Create legacy hooks.json
	legacyPath := filepath.Join(tmpDir, "hooks.json")
	if err := os.WriteFile(legacyPath, []byte(`{"hooks":{}}`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := generateSettingsHooks(tmpDir); err != nil {
		t.Fatalf("generateSettingsHooks() error = %v", err)
	}

	// Verify legacy file removed
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Error("legacy hooks.json should be removed")
	}

	// Verify settings.json created
	if _, err := os.Stat(filepath.Join(tmpDir, "settings.json")); os.IsNotExist(err) {
		t.Error("settings.json should be created")
	}
}

func TestIsPylonHookCommand(t *testing.T) {
	tests := []struct {
		command string
		want    bool
	}{
		{"pylon sync-memory --from-session", true},
		{"pylon sync-memory --incremental", true},
		{"pylon mem inject", true},
		{"my-custom-hook --on-stop", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isPylonHookCommand(tt.command); got != tt.want {
			t.Errorf("isPylonHookCommand(%q) = %v, want %v", tt.command, got, tt.want)
		}
	}
}

func setupTestWorkspace(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	pylonDir := filepath.Join(tmpDir, ".pylon")
	if err := os.MkdirAll(pylonDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pylonDir, "config.yml"), []byte("version: \"0.1\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return tmpDir
}

func TestGenerateClaudeDir_IncludesSettings(t *testing.T) {
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, ".claude")

	// Create minimal config using ParseConfig (which applies defaults)
	cfg, err := config.ParseConfig([]byte("version: \"0.1\"\n"))
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}

	err = generateClaudeDir(tmpDir, cfg, nil)
	if err != nil {
		t.Fatalf("generateClaudeDir() error = %v", err)
	}

	// Verify settings.json was created (not hooks.json)
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		t.Fatal("settings.json should be created by generateClaudeDir")
	}

	// Verify hooks.json was NOT created
	hooksPath := filepath.Join(claudeDir, "hooks.json")
	if _, err := os.Stat(hooksPath); !os.IsNotExist(err) {
		t.Error("hooks.json should NOT be created (use settings.json instead)")
	}

	// Verify settings.json has correct structure
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read settings.json: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("settings.json is invalid JSON: %v", err)
	}

	hooks, ok := settings["hooks"]
	if !ok {
		t.Error("settings.json should contain 'hooks' key")
		return
	}

	hooksMap, ok := hooks.(map[string]any)
	if !ok || len(hooksMap) == 0 {
		t.Error("settings.json hooks should contain hook definitions")
	}
}
