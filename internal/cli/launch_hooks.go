package cli

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed hooks.json
var defaultHooksJSON []byte

// settingsHookCommand represents a single command within a hook group.
type settingsHookCommand struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

// settingsHookEntry represents a hook group in .claude/settings.json.
// Each group has a matcher string and an array of hook commands.
type settingsHookEntry struct {
	Matcher string                `json:"matcher"`
	Hooks   []settingsHookCommand `json:"hooks"`
}

// generateSettingsHooks writes hook definitions into .claude/settings.json.
// It merges with any existing settings to avoid overwriting user-defined config.
// This connects the session lifecycle to pylon's memory system, solving the
// syscall.Exec memory propagation gap.
func generateSettingsHooks(claudeDir string) error {
	settingsPath := filepath.Join(claudeDir, "settings.json")

	// Load existing settings.json if present
	existing := make(map[string]any)
	if data, err := os.ReadFile(settingsPath); err == nil {
		if err := json.Unmarshal(data, &existing); err != nil {
			// If existing file is invalid JSON, start fresh but log warning
			existing = make(map[string]any)
		}
	}

	// Load pylon hook entries from embedded hooks.json
	var pylonHooks map[string][]settingsHookEntry
	if err := json.Unmarshal(defaultHooksJSON, &pylonHooks); err != nil {
		return fmt.Errorf("내장 hooks.json 파싱 실패: %w", err)
	}

	// Merge hooks: preserve non-pylon hooks, replace pylon hooks
	mergedHooks := mergeHooks(existing, pylonHooks)
	existing["hooks"] = mergedHooks

	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return fmt.Errorf("settings.json 직렬화 실패: %w", err)
	}

	if err := os.WriteFile(settingsPath, data, 0644); err != nil {
		return fmt.Errorf("settings.json 쓰기 실패: %w", err)
	}

	// Clean up legacy hooks.json if it exists
	legacyHooksPath := filepath.Join(claudeDir, "hooks.json")
	if err := os.Remove(legacyHooksPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("레거시 hooks.json 제거 실패: %w", err)
	}

	return nil
}

// isPylonHookCommand checks if a command string is a pylon-managed hook.
func isPylonHookCommand(command string) bool {
	return strings.Contains(command, "pylon sync-memory")
}

// isPylonHookGroup checks if a hook group contains any pylon-managed hook commands.
func isPylonHookGroup(entryMap map[string]any) bool {
	hooksArr, ok := entryMap["hooks"].([]any)
	if !ok {
		return false
	}
	for _, h := range hooksArr {
		if hookMap, ok := h.(map[string]any); ok {
			cmd, _ := hookMap["command"].(string)
			if isPylonHookCommand(cmd) {
				return true
			}
		}
	}
	return false
}

// mergeHooks merges pylon hook entries into existing settings, preserving
// user-defined hooks while replacing pylon-managed ones.
func mergeHooks(existing map[string]any, pylonHooks map[string][]settingsHookEntry) map[string]any {
	result := make(map[string]any)

	// Get existing hooks map if present
	var existingHooks map[string]any
	if h, ok := existing["hooks"]; ok {
		if hm, ok := h.(map[string]any); ok {
			existingHooks = hm
		}
	}

	// For each hook event type in existing, preserve non-pylon entries
	for eventType, entries := range existingHooks {
		var preserved []any
		if arr, ok := entries.([]any); ok {
			for _, entry := range arr {
				if entryMap, ok := entry.(map[string]any); ok {
					if !isPylonHookGroup(entryMap) {
						preserved = append(preserved, entry)
					}
				}
			}
		}
		if len(preserved) > 0 {
			result[eventType] = preserved
		}
	}

	// Add pylon hooks
	for eventType, entries := range pylonHooks {
		var existing []any
		if arr, ok := result[eventType]; ok {
			if a, ok := arr.([]any); ok {
				existing = a
			}
		}
		for _, entry := range entries {
			existing = append(existing, entry)
		}
		result[eventType] = existing
	}

	return result
}
