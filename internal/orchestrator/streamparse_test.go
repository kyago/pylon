package orchestrator

import (
	"testing"
)

func TestParseStreamJSON_ToolUseFiles(t *testing.T) {
	output := `{"type":"system","subtype":"init"}
{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Edit","input":{"file_path":"/tmp/project/main.go","old_string":"old","new_string":"new"}}]}}
{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t2","name":"Write","input":{"file_path":"/tmp/project/new_file.go","content":"package main"}}]}}
{"type":"assistant","message":{"content":[{"type":"text","text":"Done editing files."}]}}
{"type":"result","subtype":"success","result":"Successfully updated main.go and created new_file.go","cost_usd":0.05}`

	sr := ParseStreamJSON(output)

	if len(sr.FilesChanged) != 2 {
		t.Errorf("expected 2 files changed, got %d: %v", len(sr.FilesChanged), sr.FilesChanged)
	}

	filesMap := make(map[string]bool)
	for _, f := range sr.FilesChanged {
		filesMap[f] = true
	}
	if !filesMap["/tmp/project/main.go"] {
		t.Error("expected main.go in files changed")
	}
	if !filesMap["/tmp/project/new_file.go"] {
		t.Error("expected new_file.go in files changed")
	}

	if sr.Summary != "Successfully updated main.go and created new_file.go" {
		t.Errorf("unexpected summary: %q", sr.Summary)
	}
}

func TestParseStreamJSON_GitCommit(t *testing.T) {
	output := `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"git commit -m \"feat: add login\""}}]}}
{"type":"result","result":"Committed changes"}`

	sr := ParseStreamJSON(output)

	if len(sr.Commits) != 1 {
		t.Errorf("expected 1 commit, got %d", len(sr.Commits))
	}
}

func TestParseStreamJSON_Empty(t *testing.T) {
	sr := ParseStreamJSON("")
	if len(sr.FilesChanged) != 0 {
		t.Error("expected no files for empty output")
	}
	if sr.Summary != "" {
		t.Error("expected empty summary")
	}
}

func TestParseStreamJSON_InvalidJSON(t *testing.T) {
	output := "not json\n{invalid}\n"
	sr := ParseStreamJSON(output)
	if len(sr.FilesChanged) != 0 {
		t.Error("expected no files for invalid JSON")
	}
}

func TestParseStreamJSON_DuplicateFiles(t *testing.T) {
	output := `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Edit","input":{"file_path":"main.go","old_string":"a","new_string":"b"}}]}}
{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t2","name":"Edit","input":{"file_path":"main.go","old_string":"c","new_string":"d"}}]}}
{"type":"result","result":"Done"}`

	sr := ParseStreamJSON(output)

	if len(sr.FilesChanged) != 1 {
		t.Errorf("expected 1 unique file, got %d: %v", len(sr.FilesChanged), sr.FilesChanged)
	}
}
