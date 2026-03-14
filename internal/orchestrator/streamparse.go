package orchestrator

import (
	"encoding/json"
	"sort"
	"strings"
)

// StreamResult holds structured data extracted from Claude Code stream-json output.
type StreamResult struct {
	FilesChanged   []string
	CommitCommands []string // git commit commands found in Bash tool_use (not hashes)
	Summary        string
}

// ParseStreamJSON parses Claude Code --output-format stream-json output
// and extracts files changed, commits, and the final result summary.
//
// Stream-json format: newline-delimited JSON objects with "type" field.
// Key types:
//   - assistant messages with tool_use content (Edit, Write, Bash)
//   - result message with final summary text
func ParseStreamJSON(output string) *StreamResult {
	sr := &StreamResult{}
	filesSet := make(map[string]bool)

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] != '{' {
			continue
		}

		var msg map[string]any
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}

		msgType, _ := msg["type"].(string)

		switch msgType {
		case "assistant":
			extractToolUseFiles(msg, filesSet)
			extractBashCommits(msg, sr)
		case "result":
			if result, ok := msg["result"].(string); ok {
				sr.Summary = truncateOutput(result, 500)
			}
		}
	}

	for f := range filesSet {
		sr.FilesChanged = append(sr.FilesChanged, f)
	}
	sort.Strings(sr.FilesChanged)

	return sr
}

// extractToolUseFiles finds file paths from Edit/Write tool_use events.
func extractToolUseFiles(msg map[string]any, files map[string]bool) {
	message, ok := msg["message"].(map[string]any)
	if !ok {
		return
	}
	content, ok := message["content"].([]any)
	if !ok {
		return
	}

	for _, item := range content {
		block, ok := item.(map[string]any)
		if !ok {
			continue
		}
		blockType, _ := block["type"].(string)
		if blockType != "tool_use" {
			continue
		}

		toolName, _ := block["name"].(string)
		input, _ := block["input"].(map[string]any)
		if input == nil {
			continue
		}

		switch toolName {
		case "Edit", "Write":
			if fp, ok := input["file_path"].(string); ok && fp != "" {
				files[fp] = true
			}
		}
	}
}

// extractBashCommits finds git commit hashes from Bash tool_use events.
func extractBashCommits(msg map[string]any, sr *StreamResult) {
	message, ok := msg["message"].(map[string]any)
	if !ok {
		return
	}
	content, ok := message["content"].([]any)
	if !ok {
		return
	}

	for _, item := range content {
		block, ok := item.(map[string]any)
		if !ok {
			continue
		}
		blockType, _ := block["type"].(string)
		if blockType != "tool_use" {
			continue
		}
		toolName, _ := block["name"].(string)
		if toolName != "Bash" {
			continue
		}
		input, _ := block["input"].(map[string]any)
		if input == nil {
			continue
		}
		cmd, _ := input["command"].(string)
		if strings.Contains(cmd, "git commit") {
			sr.CommitCommands = append(sr.CommitCommands, cmd)
		}
	}
}
