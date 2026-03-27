package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kyago/pylon/internal/config"
)

func TestNewSyncMemoryCmd_Flags(t *testing.T) {
	cmd := newSyncMemoryCmd()

	flags := []struct {
		name     string
		defValue string
	}{
		{"from-session", "false"},
		{"incremental", "false"},
		{"project", ""},
		{"agent", "claude"},
		{"content", ""},
		{"file", ""},
	}

	for _, f := range flags {
		flag := cmd.Flags().Lookup(f.name)
		if flag == nil {
			t.Errorf("--%s flag not found", f.name)
			continue
		}
		if flag.DefValue != f.defValue {
			t.Errorf("--%s default = %q, want %q", f.name, flag.DefValue, f.defValue)
		}
	}
}

func TestNewSyncMemoryCmd_RequiresMode(t *testing.T) {
	cmd := newSyncMemoryCmd()
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when neither --from-session nor --incremental is set")
	}
	if !strings.Contains(err.Error(), "--from-session") {
		t.Errorf("error should mention --from-session: %v", err)
	}
}

func TestNewSyncMemoryCmd_MutuallyExclusive(t *testing.T) {
	cmd := newSyncMemoryCmd()
	cmd.SetArgs([]string{"--from-session", "--incremental"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when both --from-session and --incremental are set")
	}
	// Verify the error mentions both flags
	errMsg := err.Error()
	if !strings.Contains(errMsg, "--from-session") || !strings.Contains(errMsg, "--incremental") {
		t.Errorf("error should mention both flags: %v", err)
	}
}

func TestParseLearnings(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantLen  int
		wantNil  bool
	}{
		{"empty", "", 0, true},
		{"single line", "learned something", 1, false},
		{"multiple lines", "line1\nline2\nline3", 3, false},
		{"with empty lines", "line1\n\nline2\n\n", 2, false},
		{"with list prefixes", "- item1\n* item2\nitem3", 3, false},
		{"only whitespace", "  \n  \n  ", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseLearnings(tt.input)
			if err != nil {
				t.Fatalf("parseLearnings() error = %v", err)
			}
			if tt.wantNil && got != nil {
				t.Errorf("parseLearnings() = %v, want nil", got)
			}
			if !tt.wantNil && len(got) != tt.wantLen {
				t.Errorf("parseLearnings() len = %d, want %d", len(got), tt.wantLen)
			}
		})
	}
}

func TestParseLearnings_StripsPrefixes(t *testing.T) {
	input := "- dash item\n* star item\nplain item"
	got, err := parseLearnings(input)
	if err != nil {
		t.Fatalf("parseLearnings() error = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 items, got %d", len(got))
	}
	if got[0] != "dash item" {
		t.Errorf("got[0] = %q, want %q", got[0], "dash item")
	}
	if got[1] != "star item" {
		t.Errorf("got[1] = %q, want %q", got[1], "star item")
	}
	if got[2] != "plain item" {
		t.Errorf("got[2] = %q, want %q", got[2], "plain item")
	}
}

func TestParseLearnings_JSONPayload(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantLen int
	}{
		{
			"learnings array",
			`{"learnings": ["fact one", "fact two"]}`,
			2,
		},
		{
			"summary field",
			`{"summary": "refactored auth module"}`,
			1,
		},
		{
			"content field",
			`{"content": "added new endpoint"}`,
			1,
		},
		{
			"tool_output field",
			`{"tool_output": "file written successfully"}`,
			1,
		},
		{
			"multiple fields",
			`{"summary": "overview", "learnings": ["detail1"], "content": "body"}`,
			3,
		},
		{
			"empty json object",
			`{}`,
			1, // falls through to line splitting: "{}" is one line
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseLearnings(tt.input)
			if err != nil {
				t.Fatalf("parseLearnings() error = %v", err)
			}
			if len(got) != tt.wantLen {
				t.Errorf("parseLearnings() len = %d, want %d, got %v", len(got), tt.wantLen, got)
			}
		})
	}
}

func TestParseLearnings_EmptyJSONFallsThrough(t *testing.T) {
	// Empty JSON should fall through to plain text, resulting in nil
	got, err := parseLearnings(`{}`)
	if err != nil {
		t.Fatalf("parseLearnings() error = %v", err)
	}
	// {} has no usable fields, falls through to line splitting which yields ["{}", nil]
	// Actually {} will be parsed as JSON with no fields, return nil from tryParseJSONLearnings,
	// then fall through to line split which yields ["{}"]
	if got == nil {
		t.Error("expected fallthrough to produce result from line splitting")
	}
}

func TestTryParseJSONLearnings(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantLen int
	}{
		{"valid learnings", `{"learnings": ["a", "b"]}`, 2},
		{"invalid json", `not json`, 0},
		{"empty object", `{}`, 0},
		{"summary only", `{"summary": "test"}`, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tryParseJSONLearnings(tt.input)
			if len(got) != tt.wantLen {
				t.Errorf("tryParseJSONLearnings() len = %d, want %d", len(got), tt.wantLen)
			}
		})
	}
}

func TestTryParseToolUsePayload(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantFile    string
		wantContent string
		wantEmpty   bool
	}{
		{
			name:      "Write tool",
			input:     `{"tool_name": "Write", "tool_input": {"file_path": "src/main.go", "content": "package main"}}`,
			wantFile:  "src/main.go",
			wantEmpty: false,
		},
		{
			name:      "Edit tool",
			input:     `{"tool_name": "Edit", "tool_input": {"file_path": "src/main.go", "old_string": "foo", "new_string": "bar"}}`,
			wantFile:  "src/main.go",
			wantEmpty: false,
		},
		{
			name:      "invalid json",
			input:     `not json`,
			wantFile:  "",
			wantEmpty: true,
		},
		{
			name:      "empty object",
			input:     `{}`,
			wantFile:  "",
			wantEmpty: true,
		},
		{
			name:      "unknown tool with file",
			input:     `{"tool_name": "CustomTool", "tool_input": {"file_path": "config.yml"}}`,
			wantFile:  "config.yml",
			wantEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotFile, gotContent := tryParseToolUsePayload(tt.input)
			if gotFile != tt.wantFile {
				t.Errorf("tryParseToolUsePayload() file = %q, want %q", gotFile, tt.wantFile)
			}
			if tt.wantEmpty && gotContent != "" {
				t.Errorf("tryParseToolUsePayload() content should be empty, got %q", gotContent)
			}
			if !tt.wantEmpty && gotContent == "" {
				t.Errorf("tryParseToolUsePayload() content should not be empty")
			}
		})
	}
}

func TestTryParseToolUsePayload_WriteContent(t *testing.T) {
	input := `{"tool_name": "Write", "tool_input": {"file_path": "src/main.go", "content": "package main\n\nfunc main() {}"}}`
	gotFile, gotContent := tryParseToolUsePayload(input)
	if gotFile != "src/main.go" {
		t.Errorf("file = %q, want %q", gotFile, "src/main.go")
	}
	if !strings.Contains(gotContent, "[Write]") {
		t.Errorf("content should contain '[Write]' prefix, got: %q", gotContent)
	}
	if !strings.Contains(gotContent, "src/main.go") {
		t.Errorf("content should contain file path, got: %q", gotContent)
	}
}

func TestTryParseToolUsePayload_EditContent(t *testing.T) {
	input := `{"tool_name": "Edit", "tool_input": {"file_path": "src/main.go", "old_string": "oldCode()", "new_string": "newCode()"}}`
	_, gotContent := tryParseToolUsePayload(input)
	if !strings.Contains(gotContent, "[Edit]") {
		t.Errorf("content should contain '[Edit]' prefix, got: %q", gotContent)
	}
	if !strings.Contains(gotContent, "oldCode()") {
		t.Errorf("content should contain old string, got: %q", gotContent)
	}
	if !strings.Contains(gotContent, "newCode()") {
		t.Errorf("content should contain new string, got: %q", gotContent)
	}
}

func TestTryParseToolUsePayload_LargeContentTruncated(t *testing.T) {
	largeContent := strings.Repeat("x", 1000)
	input := fmt.Sprintf(`{"tool_name": "Write", "tool_input": {"file_path": "big.go", "content": %q}}`, largeContent)
	_, gotContent := tryParseToolUsePayload(input)
	if !strings.Contains(gotContent, "truncated") {
		t.Errorf("large content should be truncated, got length %d", len(gotContent))
	}
}

func TestResolveProject_MultiProject(t *testing.T) {
	// In multi-project workspaces, resolveProject should fall back to workspace name
	// (not error) because hooks cannot pass --project dynamically
	tmpDir := t.TempDir()
	// DiscoverProjects will find 0 projects in an empty dir, so we test the fallback directly
	got, err := resolveProject(tmpDir, "", "")
	if err != nil {
		t.Fatalf("resolveProject() should not error: %v", err)
	}
	expected := filepath.Base(tmpDir)
	if got != expected {
		t.Errorf("resolveProject() = %q, want %q", got, expected)
	}
}

func TestBuildIncrementalKey(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		wantPfx  string
	}{
		{"empty path", "", "20"},           // starts with timestamp
		{"simple file", "main.go", "main"}, // starts with sanitized filename
		{"nested path", "src/pkg/file.go", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildIncrementalKey(tt.filePath)
			if tt.wantPfx != "" && !strings.HasPrefix(got, tt.wantPfx) {
				t.Errorf("buildIncrementalKey(%q) = %q, want prefix %q", tt.filePath, got, tt.wantPfx)
			}
			// Should not have "change/" prefix (category is stored separately)
			if strings.HasPrefix(got, "change/") {
				t.Errorf("buildIncrementalKey(%q) should not have 'change/' prefix, got: %s", tt.filePath, got)
			}
			// Should contain nanosecond timestamp
			if !strings.Contains(got, ".") {
				t.Errorf("buildIncrementalKey(%q) should contain nanosecond timestamp, got: %s", tt.filePath, got)
			}
		})
	}
}

func TestBuildIncrementalKey_CrossPlatformPaths(t *testing.T) {
	// Test both forward and back slash normalization
	got := buildIncrementalKey("src/pkg/file.go")
	if strings.Contains(got, "/pkg/") {
		t.Errorf("forward slashes should be normalized: %s", got)
	}

	got = buildIncrementalKey("src\\pkg\\file.go")
	if strings.Contains(got, "\\") {
		t.Errorf("back slashes should be normalized: %s", got)
	}
}

func TestBuildIncrementalKey_LongPath(t *testing.T) {
	longPath := strings.Repeat("a", 100) + "/file.go"
	got := buildIncrementalKey(longPath)
	// Key should be truncated and not excessively long
	parts := strings.Split(got, "/")
	if len(parts) < 2 {
		t.Errorf("expected at least 2 path segments, got %d in %q", len(parts), got)
	}
}

func TestBuildIncrementalKey_UniqueTimestamp(t *testing.T) {
	// Two keys generated in quick succession should be different due to nanoseconds
	key1 := buildIncrementalKey("test.go")
	key2 := buildIncrementalKey("test.go")
	if key1 == key2 {
		t.Errorf("keys should be unique even in rapid succession: %q == %q", key1, key2)
	}
}

func TestResolveProject(t *testing.T) {
	// When project flag is explicitly set, use it directly
	got, err := resolveProject("/tmp/test", "myproject", "")
	if err != nil {
		t.Fatalf("resolveProject() error = %v", err)
	}
	if got != "myproject" {
		t.Errorf("resolveProject() = %q, want %q", got, "myproject")
	}
}

func TestResolveProject_FallbackToDirName(t *testing.T) {
	tmpDir := t.TempDir()
	got, err := resolveProject(tmpDir, "", "")
	if err != nil {
		t.Fatalf("resolveProject() error = %v", err)
	}
	// Should fall back to directory base name (no projects discovered)
	expected := filepath.Base(tmpDir)
	if got != expected {
		t.Errorf("resolveProject() = %q, want %q", got, expected)
	}
}

func TestResolveProject_FilePathInference(t *testing.T) {
	// Setup: create a workspace with two projects
	tmpDir := t.TempDir()
	projADir := filepath.Join(tmpDir, "project-a")
	projBDir := filepath.Join(tmpDir, "project-b")
	for _, d := range []string{projADir, projBDir} {
		pylonDir := filepath.Join(d, ".pylon")
		if err := os.MkdirAll(pylonDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pylonDir, "config.yml"), []byte("version: \"0.1\"\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	// Create workspace-level .pylon/config.yml with projects
	wsPylon := filepath.Join(tmpDir, ".pylon")
	if err := os.MkdirAll(wsPylon, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wsPylon, "config.yml"), []byte("version: \"0.1\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Test: file in project-a should resolve to project-a
	filePath := filepath.Join(projADir, "src", "main.go")
	got, err := resolveProject(tmpDir, "", filePath)
	if err != nil {
		t.Fatalf("resolveProject() error = %v", err)
	}
	if got != "project-a" {
		t.Errorf("resolveProject() = %q, want %q", got, "project-a")
	}

	// Test: file in project-b should resolve to project-b
	filePath = filepath.Join(projBDir, "pkg", "util.go")
	got, err = resolveProject(tmpDir, "", filePath)
	if err != nil {
		t.Fatalf("resolveProject() error = %v", err)
	}
	if got != "project-b" {
		t.Errorf("resolveProject() = %q, want %q", got, "project-b")
	}

	// Test: file outside both projects should fallback to workspace name
	filePath = filepath.Join(tmpDir, "README.md")
	got, err = resolveProject(tmpDir, "", filePath)
	if err != nil {
		t.Fatalf("resolveProject() error = %v", err)
	}
	expected := filepath.Base(tmpDir)
	if got != expected {
		t.Errorf("resolveProject() = %q, want %q (workspace fallback)", got, expected)
	}

	// Test: relative path should resolve against workspace root
	relPath := "project-a/src/handler.go"
	got, err = resolveProject(tmpDir, "", relPath)
	if err != nil {
		t.Fatalf("resolveProject() error = %v", err)
	}
	if got != "project-a" {
		t.Errorf("resolveProject() with relative path = %q, want %q", got, "project-a")
	}
}

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

	// Verify Stop hook exists with correct structure
	stopHooks, ok := hooksMap["Stop"]
	if !ok {
		t.Fatal("Stop hook not found")
	}
	stopArr, ok := stopHooks.([]any)
	if !ok || len(stopArr) == 0 {
		t.Fatal("Stop hook should be a non-empty array")
	}
	stopGroup, ok := stopArr[0].(map[string]any)
	if !ok {
		t.Fatal("Stop hook group should be a map")
	}
	// Verify matcher is a string
	stopMatcher, ok := stopGroup["matcher"].(string)
	if !ok {
		t.Fatal("Stop hook matcher should be a string")
	}
	if stopMatcher != "*" {
		t.Errorf("Stop hook matcher = %q, want \"*\"", stopMatcher)
	}
	// Verify hooks array
	stopInnerHooks, ok := stopGroup["hooks"].([]any)
	if !ok || len(stopInnerHooks) == 0 {
		t.Fatal("Stop hook group should have a non-empty 'hooks' array")
	}
	stopCmd, ok := stopInnerHooks[0].(map[string]any)
	if !ok {
		t.Fatal("Stop inner hook should be a map")
	}
	if stopCmd["type"] != "command" {
		t.Errorf("Stop hook type = %v, want 'command'", stopCmd["type"])
	}
	cmd, _ := stopCmd["command"].(string)
	if !strings.Contains(cmd, "pylon sync-memory") || !strings.Contains(cmd, "--from-session") {
		t.Errorf("Stop hook command should contain 'pylon sync-memory --from-session', got: %s", cmd)
	}

	// Verify PostToolUse hook exists with correct structure
	postHooks, ok := hooksMap["PostToolUse"]
	if !ok {
		t.Fatal("PostToolUse hook not found")
	}
	postArr, ok := postHooks.([]any)
	if !ok || len(postArr) == 0 {
		t.Fatal("PostToolUse hook should be a non-empty array")
	}
	postGroup, ok := postArr[0].(map[string]any)
	if !ok {
		t.Fatal("PostToolUse hook group should be a map")
	}
	// Verify matcher is a string
	postMatcher, ok := postGroup["matcher"].(string)
	if !ok {
		t.Fatal("PostToolUse hook matcher should be a string")
	}
	if postMatcher != "Edit|Write" {
		t.Errorf("PostToolUse hook matcher = %q, want 'Edit|Write'", postMatcher)
	}
	// Verify hooks array
	postInnerHooks, ok := postGroup["hooks"].([]any)
	if !ok || len(postInnerHooks) == 0 {
		t.Fatal("PostToolUse hook group should have a non-empty 'hooks' array")
	}
	postCmd, ok := postInnerHooks[0].(map[string]any)
	if !ok {
		t.Fatal("PostToolUse inner hook should be a map")
	}
	if postCmd["type"] != "command" {
		t.Errorf("PostToolUse hook type = %v, want 'command'", postCmd["type"])
	}

	// Verify no description field (not in Claude Code spec)
	if _, hasDesc := stopGroup["description"]; hasDesc {
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

	// Stop should have user hook group + pylon hook group
	stopHooks := hooks["Stop"].([]any)
	if len(stopHooks) != 2 {
		t.Errorf("Stop should have 2 entries (user + pylon), got %d", len(stopHooks))
	}

	// Verify user's custom hook group is first (preserved)
	userGroup := stopHooks[0].(map[string]any)
	userInnerHooks := userGroup["hooks"].([]any)
	userCmd := userInnerHooks[0].(map[string]any)
	if userCmd["command"] != "my-custom-hook --on-stop" {
		t.Error("user's custom Stop hook should be preserved")
	}

	// Verify pylon hook group is second (added)
	pylonGroup := stopHooks[1].(map[string]any)
	pylonInnerHooks := pylonGroup["hooks"].([]any)
	pylonCmd := pylonInnerHooks[0].(map[string]any)
	cmd, _ := pylonCmd["command"].(string)
	if !strings.Contains(cmd, "pylon sync-memory") {
		t.Error("pylon Stop hook should be added")
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
		{"my-custom-hook --on-stop", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isPylonHookCommand(tt.command); got != tt.want {
			t.Errorf("isPylonHookCommand(%q) = %v, want %v", tt.command, got, tt.want)
		}
	}
}

func TestRunSyncFromSession_NoWorkspace(t *testing.T) {
	// Point to a non-workspace directory
	oldWorkspace := flagWorkspace
	flagWorkspace = t.TempDir()
	defer func() { flagWorkspace = oldWorkspace }()

	err := runSyncFromSession("test-project", "test-agent", "some learning")
	if err == nil {
		t.Fatal("expected error for non-workspace directory")
	}
}

func TestRunSyncIncremental_NoWorkspace(t *testing.T) {
	// Point to a non-workspace directory
	oldWorkspace := flagWorkspace
	flagWorkspace = t.TempDir()
	defer func() { flagWorkspace = oldWorkspace }()

	err := runSyncIncremental("test-project", "test-agent", "file.go", "some content")
	if err == nil {
		t.Fatal("expected error for non-workspace directory")
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

func TestRunSyncFromSession_WithWorkspace(t *testing.T) {
	tmpDir := setupTestWorkspace(t)

	oldWorkspace := flagWorkspace
	flagWorkspace = tmpDir
	defer func() { flagWorkspace = oldWorkspace }()

	err := runSyncFromSession("test-project", "test-agent", "learned something important\nand another thing")
	if err != nil {
		t.Fatalf("runSyncFromSession() error = %v", err)
	}
}

func TestRunSyncFromSession_EmptyContent(t *testing.T) {
	tmpDir := setupTestWorkspace(t)

	oldWorkspace := flagWorkspace
	flagWorkspace = tmpDir
	defer func() { flagWorkspace = oldWorkspace }()

	// Empty content should succeed without error (just skip)
	err := runSyncFromSession("test-project", "test-agent", "")
	if err != nil {
		t.Fatalf("runSyncFromSession() error = %v", err)
	}
}

func TestRunSyncIncremental_WithWorkspace(t *testing.T) {
	tmpDir := setupTestWorkspace(t)

	oldWorkspace := flagWorkspace
	flagWorkspace = tmpDir
	defer func() { flagWorkspace = oldWorkspace }()

	err := runSyncIncremental("test-project", "test-agent", "src/main.go", "refactored auth module")
	if err != nil {
		t.Fatalf("runSyncIncremental() error = %v", err)
	}
}

func TestRunSyncIncremental_EmptyContent(t *testing.T) {
	tmpDir := setupTestWorkspace(t)

	oldWorkspace := flagWorkspace
	flagWorkspace = tmpDir
	defer func() { flagWorkspace = oldWorkspace }()

	// Empty content should succeed without error (just skip)
	err := runSyncIncremental("test-project", "test-agent", "file.go", "")
	if err != nil {
		t.Fatalf("runSyncIncremental() error = %v", err)
	}
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
