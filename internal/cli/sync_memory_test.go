package cli

import (
	"encoding/json"
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

func TestBuildIncrementalKey(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		wantPfx  string
	}{
		{"empty path", "", "change/"},
		{"simple file", "main.go", "change/main-go/"},
		{"nested path", "src/pkg/file.go", "change/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildIncrementalKey(tt.filePath)
			if !strings.HasPrefix(got, tt.wantPfx) {
				t.Errorf("buildIncrementalKey(%q) = %q, want prefix %q", tt.filePath, got, tt.wantPfx)
			}
			// Should contain a timestamp portion
			if !strings.Contains(got, "-") {
				t.Errorf("buildIncrementalKey(%q) should contain timestamp", tt.filePath)
			}
		})
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

func TestResolveProject(t *testing.T) {
	// When project flag is explicitly set, use it directly
	got, err := resolveProject("/tmp/test", "myproject")
	if err != nil {
		t.Fatalf("resolveProject() error = %v", err)
	}
	if got != "myproject" {
		t.Errorf("resolveProject() = %q, want %q", got, "myproject")
	}
}

func TestResolveProject_FallbackToDirName(t *testing.T) {
	tmpDir := t.TempDir()
	got, err := resolveProject(tmpDir, "")
	if err != nil {
		t.Fatalf("resolveProject() error = %v", err)
	}
	// Should fall back to directory base name
	expected := filepath.Base(tmpDir)
	if got != expected {
		t.Errorf("resolveProject() = %q, want %q", got, expected)
	}
}

func TestGenerateHooksJSON(t *testing.T) {
	tmpDir := t.TempDir()

	if err := generateHooksJSON(tmpDir); err != nil {
		t.Fatalf("generateHooksJSON() error = %v", err)
	}

	hooksPath := filepath.Join(tmpDir, "hooks.json")
	data, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("failed to read hooks.json: %v", err)
	}

	// Parse and verify structure
	var hooks hooksConfig
	if err := json.Unmarshal(data, &hooks); err != nil {
		t.Fatalf("failed to parse hooks.json: %v", err)
	}

	// Verify Stop hook exists
	stopHooks, ok := hooks.Hooks["Stop"]
	if !ok || len(stopHooks) == 0 {
		t.Fatal("Stop hook not found")
	}
	if !strings.Contains(stopHooks[0].Command, "pylon sync-memory") {
		t.Errorf("Stop hook command should contain 'pylon sync-memory', got: %s", stopHooks[0].Command)
	}
	if !strings.Contains(stopHooks[0].Command, "--from-session") {
		t.Errorf("Stop hook command should contain '--from-session', got: %s", stopHooks[0].Command)
	}

	// Verify PostToolUse hook exists
	postHooks, ok := hooks.Hooks["PostToolUse"]
	if !ok || len(postHooks) == 0 {
		t.Fatal("PostToolUse hook not found")
	}
	if !strings.Contains(postHooks[0].Command, "pylon sync-memory") {
		t.Errorf("PostToolUse hook command should contain 'pylon sync-memory', got: %s", postHooks[0].Command)
	}
	if !strings.Contains(postHooks[0].Command, "--incremental") {
		t.Errorf("PostToolUse hook command should contain '--incremental', got: %s", postHooks[0].Command)
	}

	// Verify matcher
	if postHooks[0].Matcher == nil {
		t.Fatal("PostToolUse hook should have a matcher")
	}
	if postHooks[0].Matcher.ToolName != "Write|Edit" {
		t.Errorf("PostToolUse matcher.tool_name = %q, want %q", postHooks[0].Matcher.ToolName, "Write|Edit")
	}
}

func TestGenerateHooksJSON_ValidJSON(t *testing.T) {
	tmpDir := t.TempDir()

	if err := generateHooksJSON(tmpDir); err != nil {
		t.Fatalf("generateHooksJSON() error = %v", err)
	}

	hooksPath := filepath.Join(tmpDir, "hooks.json")
	data, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("failed to read hooks.json: %v", err)
	}

	// Verify it's valid JSON
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("hooks.json is not valid JSON: %v", err)
	}

	// Verify top-level "hooks" key exists
	if _, ok := raw["hooks"]; !ok {
		t.Error("hooks.json should have a top-level 'hooks' key")
	}
}

func TestGenerateHooksJSON_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()

	// Generate twice
	if err := generateHooksJSON(tmpDir); err != nil {
		t.Fatalf("first generateHooksJSON() error = %v", err)
	}
	first, _ := os.ReadFile(filepath.Join(tmpDir, "hooks.json"))

	if err := generateHooksJSON(tmpDir); err != nil {
		t.Fatalf("second generateHooksJSON() error = %v", err)
	}
	second, _ := os.ReadFile(filepath.Join(tmpDir, "hooks.json"))

	if string(first) != string(second) {
		t.Error("generateHooksJSON should be idempotent")
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

func TestGenerateClaudeDir_IncludesHooks(t *testing.T) {
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, ".claude")

	// Create minimal config using ParseConfig (which applies defaults)
	cfg, err := config.ParseConfig([]byte("version: \"0.1\"\n"))
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}

	err = generateClaudeDir(tmpDir, cfg, nil, "")
	if err != nil {
		t.Fatalf("generateClaudeDir() error = %v", err)
	}

	// Verify hooks.json was created
	hooksPath := filepath.Join(claudeDir, "hooks.json")
	if _, err := os.Stat(hooksPath); os.IsNotExist(err) {
		t.Fatal("hooks.json should be created by generateClaudeDir")
	}

	// Verify it has correct structure
	data, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("failed to read hooks.json: %v", err)
	}

	var hooks hooksConfig
	if err := json.Unmarshal(data, &hooks); err != nil {
		t.Fatalf("hooks.json is invalid JSON: %v", err)
	}

	if len(hooks.Hooks) == 0 {
		t.Error("hooks.json should contain hook definitions")
	}
}
