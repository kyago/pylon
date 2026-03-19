package protocol

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileTransport_WriteAndReadTask(t *testing.T) {
	dir := t.TempDir()
	ft := NewFileTransport(filepath.Join(dir, "inbox"), filepath.Join(dir, "outbox"))

	msg := NewMessage(MsgTaskAssign, "orchestrator", "backend-dev")
	msg.Context = &MsgContext{TaskID: "task-001"}
	msg.Body = TaskAssignBody{
		TaskID:      "task-001",
		Description: "implement login",
		Branch:      "feat/login",
	}

	if err := ft.WriteTask("backend-dev", msg); err != nil {
		t.Fatalf("WriteTask: %v", err)
	}

	// Verify file exists
	taskFile := filepath.Join(dir, "inbox", "backend-dev", "task-001.task.json")
	if _, err := os.Stat(taskFile); os.IsNotExist(err) {
		t.Fatal("task file not created")
	}
}

func TestFileTransport_WriteAndReadResult(t *testing.T) {
	dir := t.TempDir()
	ft := NewFileTransport(filepath.Join(dir, "inbox"), filepath.Join(dir, "outbox"))

	msg := NewMessage(MsgResult, "backend-dev", "orchestrator")
	msg.Context = &MsgContext{TaskID: "task-001"}
	msg.Body = map[string]any{
		"task_id": "task-001",
		"status":  "completed",
		"summary": "login implemented",
	}

	if err := ft.WriteResult("backend-dev", msg); err != nil {
		t.Fatalf("WriteResult: %v", err)
	}

	// Read back
	resultFile := filepath.Join(dir, "outbox", "backend-dev", "task-001.result.json")
	env, err := ft.ReadResult(resultFile)
	if err != nil {
		t.Fatalf("ReadResult: %v", err)
	}

	if env.Type != MsgResult {
		t.Fatalf("expected type %s, got %s", MsgResult, env.Type)
	}
}

func TestFileTransport_ScanResults(t *testing.T) {
	dir := t.TempDir()
	ft := NewFileTransport(filepath.Join(dir, "inbox"), filepath.Join(dir, "outbox"))

	// Write two results
	for _, agent := range []string{"agent-a", "agent-b"} {
		msg := NewMessage(MsgResult, agent, "orchestrator")
		msg.Context = &MsgContext{TaskID: "task-" + agent}
		msg.Body = map[string]any{"task_id": "task-" + agent, "status": "completed"}
		if err := ft.WriteResult(agent, msg); err != nil {
			t.Fatalf("WriteResult for %s: %v", agent, err)
		}
	}

	results, err := ft.ScanResults()
	if err != nil {
		t.Fatalf("ScanResults: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestFileTransport_ScanResults_Empty(t *testing.T) {
	dir := t.TempDir()
	ft := NewFileTransport(filepath.Join(dir, "inbox"), filepath.Join(dir, "outbox"))

	results, err := ft.ScanResults()
	if err != nil {
		t.Fatalf("ScanResults: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}
