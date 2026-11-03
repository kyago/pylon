package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// isPylonHookCommand checks if a command string is the hook earlier releases installed.
func isPylonHookCommand(command string) bool {
	return strings.Contains(command, "pylon mem inject")
}

// removePylonHooks drops the SessionStart hook earlier releases wrote into
// .claude/settings.json and reports whether the file changed. Other hooks in the same
// group and every other setting stay. A missing file is fine; an unreadable one is left
// as it is and reported.
func removePylonHooks(claudeDir string) (bool, error) {
	settingsPath := filepath.Join(claudeDir, "settings.json")
	data, err := os.ReadFile(settingsPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return false, fmt.Errorf("settings.json 파싱 실패: %w", err)
	}
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		return false, nil
	}
	removed := false
	for event, entries := range hooks {
		groups, ok := entries.([]any)
		if !ok {
			continue
		}
		var kept []any
		for _, entry := range groups {
			group, ok := entry.(map[string]any)
			commands, isList := group["hooks"].([]any)
			if !ok || !isList {
				kept = append(kept, entry)
				continue
			}
			var keptCommands []any
			for _, item := range commands {
				command, _ := item.(map[string]any)
				text, _ := command["command"].(string)
				if isPylonHookCommand(text) {
					removed = true
					continue
				}
				keptCommands = append(keptCommands, item)
			}
			if len(keptCommands) > 0 {
				group["hooks"] = keptCommands
				kept = append(kept, group)
			}
		}
		if len(kept) > 0 {
			hooks[event] = kept
		} else {
			delete(hooks, event)
		}
	}
	if !removed {
		return false, nil
	}
	if len(hooks) == 0 {
		delete(settings, "hooks")
	}
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return false, fmt.Errorf("settings.json 직렬화 실패: %w", err)
	}
	if err := os.WriteFile(settingsPath, out, 0o644); err != nil {
		return false, fmt.Errorf("settings.json 쓰기 실패: %w", err)
	}
	return true, nil
}
