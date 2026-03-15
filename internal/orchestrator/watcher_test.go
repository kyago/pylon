package orchestrator

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/kyago/pylon/internal/protocol"
	"github.com/kyago/pylon/internal/store"
)

func newTestStoreForWatcher(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.NewStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}
	if err := s.Migrate(); err != nil {
		t.Fatalf("failed to migrate test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOutboxWatcher_PollOnce_Empty(t *testing.T) {
	dir := t.TempDir()
	w := NewOutboxWatcher(dir, time.Second, nil)

	results, err := w.PollOnce()
	if err != nil {
		t.Fatalf("PollOnce failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestOutboxWatcher_PollOnce_NonexistentDir(t *testing.T) {
	w := NewOutboxWatcher("/nonexistent/outbox", time.Second, nil)

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

	w := NewOutboxWatcher(dir, time.Second, nil)
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

func TestOutboxWatcher_PollOnce_SkipsProcessed_Store(t *testing.T) {
	dir := t.TempDir()
	s := newTestStoreForWatcher(t)

	msg := protocol.NewMessage(protocol.MsgResult, "backend-dev", "orchestrator")
	msg.Context = &protocol.MsgContext{TaskID: "task-002"}
	msg.Body = protocol.ResultBody{TaskID: "task-002", Status: "completed"}
	if err := protocol.WriteResult(dir, "backend-dev", msg); err != nil {
		t.Fatalf("WriteResult failed: %v", err)
	}

	// Mark as processed via Store.Enqueue with acked status
	s.Enqueue(&store.QueuedMessage{
		Type:      "result",
		FromAgent: "backend-dev",
		ToAgent:   "orchestrator",
		Body:      `{"task_id":"task-002"}`,
		Status:    "acked",
	})

	w := NewOutboxWatcher(dir, time.Second, s)
	results, err := w.PollOnce()
	if err != nil {
		t.Fatalf("PollOnce failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results (processed), got %d", len(results))
	}
}

func TestOutboxWatcher_PollOnce_SkipsProcessed_DoneFallback(t *testing.T) {
	dir := t.TempDir()

	msg := protocol.NewMessage(protocol.MsgResult, "backend-dev", "orchestrator")
	msg.Context = &protocol.MsgContext{TaskID: "task-003"}
	msg.Body = protocol.ResultBody{TaskID: "task-003", Status: "completed"}
	if err := protocol.WriteResult(dir, "backend-dev", msg); err != nil {
		t.Fatalf("WriteResult failed: %v", err)
	}

	// Mark as processed via .done marker (fallback when Store is nil)
	doneFile := dir + "/backend-dev/task-003.result.json.done"
	os.WriteFile(doneFile, []byte{}, 0644)

	w := NewOutboxWatcher(dir, time.Second, nil)
	results, err := w.PollOnce()
	if err != nil {
		t.Fatalf("PollOnce failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results (processed via .done), got %d", len(results))
	}
}

func TestOutboxWatcher_WaitForResults(t *testing.T) {
	dir := t.TempDir()

	// Write result before waiting
	msg := protocol.NewMessage(protocol.MsgResult, "architect", "orchestrator")
	msg.Context = &protocol.MsgContext{TaskID: "task-004"}
	msg.Body = protocol.ResultBody{TaskID: "task-004", Status: "completed"}
	if err := protocol.WriteResult(dir, "architect", msg); err != nil {
		t.Fatalf("WriteResult failed: %v", err)
	}

	w := NewOutboxWatcher(dir, 50*time.Millisecond, nil)

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
	w := NewOutboxWatcher(dir, 50*time.Millisecond, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := w.WaitForResults(ctx, []string{"nonexistent"})
	if err == nil {
		t.Error("expected context deadline error")
	}
}

func TestOutboxWatcher_WaitForResults_Empty(t *testing.T) {
	dir := t.TempDir()
	w := NewOutboxWatcher(dir, time.Second, nil)

	results, err := w.WaitForResults(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestNewOutboxWatcher_DefaultInterval(t *testing.T) {
	w := NewOutboxWatcher("/tmp", 0, nil)
	if w.PollInterval != 2*time.Second {
		t.Errorf("default interval = %v, want 2s", w.PollInterval)
	}
}
