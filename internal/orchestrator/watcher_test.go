package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kyago/pylon/internal/protocol"
)

func TestOutboxWatcher_PollOnce_Empty(t *testing.T) {
	dir := t.TempDir()
	w := NewOutboxWatcher(dir, time.Second)

	results, err := w.PollOnce()
	if err != nil {
		t.Fatalf("PollOnce failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestOutboxWatcher_PollOnce_NonexistentDir(t *testing.T) {
	w := NewOutboxWatcher("/nonexistent/outbox", time.Second)

	results, err := w.PollOnce()
	if err != nil {
		t.Fatalf("PollOnce should not error for nonexistent: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestOutboxWatcher_PollOnce_FindsResults(t *testing.T) {
	dir := t.TempDir()

	// Create a result using protocol.WriteResult
	msg := protocol.NewMessage(protocol.MsgResult, "backend-dev", "orchestrator")
	msg.Subject = "Task completed"
	msg.Context = &protocol.MsgContext{TaskID: "task-001"}
	msg.Body = protocol.ResultBody{
		TaskID: "task-001",
		Status: "completed",
	}
	if err := protocol.WriteResult(dir, "backend-dev", msg); err != nil {
		t.Fatalf("WriteResult failed: %v", err)
	}

	w := NewOutboxWatcher(dir, time.Second)
	results, err := w.PollOnce()
	if err != nil {
		t.Fatalf("PollOnce failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].AgentName != "backend-dev" {
		t.Errorf("agent name = %q, want backend-dev", results[0].AgentName)
	}
}

func TestOutboxWatcher_PollOnce_SkipsProcessed(t *testing.T) {
	dir := t.TempDir()

	msg := protocol.NewMessage(protocol.MsgResult, "backend-dev", "orchestrator")
	msg.Context = &protocol.MsgContext{TaskID: "task-002"}
	msg.Body = protocol.ResultBody{TaskID: "task-002", Status: "completed"}
	if err := protocol.WriteResult(dir, "backend-dev", msg); err != nil {
		t.Fatalf("WriteResult failed: %v", err)
	}

	// Mark as processed
	markProcessed(dir+"/backend-dev", "task-002.result.json")

	w := NewOutboxWatcher(dir, time.Second)
	results, err := w.PollOnce()
	if err != nil {
		t.Fatalf("PollOnce failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results (processed), got %d", len(results))
	}
}

func TestOutboxWatcher_WaitForResults(t *testing.T) {
	dir := t.TempDir()

	// Write result before waiting
	msg := protocol.NewMessage(protocol.MsgResult, "architect", "orchestrator")
	msg.Context = &protocol.MsgContext{TaskID: "task-003"}
	msg.Body = protocol.ResultBody{TaskID: "task-003", Status: "completed"}
	if err := protocol.WriteResult(dir, "architect", msg); err != nil {
		t.Fatalf("WriteResult failed: %v", err)
	}

	w := NewOutboxWatcher(dir, 50*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	results, err := w.WaitForResults(ctx, []string{"architect"})
	if err != nil {
		t.Fatalf("WaitForResults failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].AgentName != "architect" {
		t.Errorf("agent = %q, want architect", results[0].AgentName)
	}
}

func TestOutboxWatcher_WaitForResults_Timeout(t *testing.T) {
	dir := t.TempDir()
	w := NewOutboxWatcher(dir, 50*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := w.WaitForResults(ctx, []string{"nonexistent"})
	if err == nil {
		t.Error("expected context deadline error")
	}
}

func TestOutboxWatcher_WaitForResults_Empty(t *testing.T) {
	dir := t.TempDir()
	w := NewOutboxWatcher(dir, time.Second)

	results, err := w.WaitForResults(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestNewOutboxWatcher_DefaultInterval(t *testing.T) {
	w := NewOutboxWatcher("/tmp", 0)
	if w.PollInterval != 2*time.Second {
		t.Errorf("default interval = %v, want 2s", w.PollInterval)
	}
}
