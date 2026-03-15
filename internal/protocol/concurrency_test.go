package protocol

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestWriteTask_ConcurrentWrites verifies that multiple goroutines can
// simultaneously write tasks for the same agent without file corruption.
func TestWriteTask_ConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	inboxDir := filepath.Join(dir, "inbox")

	const numGoroutines = 10
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			taskID := fmt.Sprintf("task-%d", id)
			msg := NewMessage(MsgTaskAssign, "orchestrator", "backend-dev")
			msg.Subject = fmt.Sprintf("Task %d", id)
			msg.Body = map[string]any{
				"task_id":     taskID,
				"description": fmt.Sprintf("concurrent task %d", id),
			}
			msg.Context = &MsgContext{TaskID: taskID}

			if err := WriteTask(inboxDir, "backend-dev", msg); err != nil {
				t.Errorf("goroutine %d: WriteTask failed: %v", id, err)
			}
		}(i)
	}

	wg.Wait()

	// 모든 파일이 정상 생성되었는지 확인
	agentDir := filepath.Join(inboxDir, "backend-dev")
	entries, err := os.ReadDir(agentDir)
	if err != nil {
		t.Fatalf("failed to read agent dir: %v", err)
	}

	taskFiles := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".task.json") {
			taskFiles++

			// 각 파일이 유효한 JSON인지 확인
			path := filepath.Join(agentDir, e.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				t.Errorf("failed to read %s: %v", e.Name(), err)
				continue
			}
			env, err := Unmarshal(data)
			if err != nil {
				t.Errorf("failed to unmarshal %s: %v", e.Name(), err)
				continue
			}
			if env.Type != MsgTaskAssign {
				t.Errorf("file %s: expected type %q, got %q", e.Name(), MsgTaskAssign, env.Type)
			}
		}
	}

	if taskFiles != numGoroutines {
		t.Errorf("expected %d task files, got %d", numGoroutines, taskFiles)
	}
}

// TestScanOutbox_ConcurrentWriteAndScan verifies that WriteResult and
// ScanOutbox can operate concurrently without errors.
func TestScanOutbox_ConcurrentWriteAndScan(t *testing.T) {
	dir := t.TempDir()
	outboxDir := filepath.Join(dir, "outbox")

	const numWriters = 5
	const numScanners = 3
	var wg sync.WaitGroup

	// WriteResult와 ScanOutbox를 동시에 수행
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			agentName := fmt.Sprintf("agent-%d", id)
			taskID := fmt.Sprintf("task-%d", id)

			msg := NewMessage(MsgResult, agentName, "orchestrator")
			msg.Body = map[string]any{
				"task_id": taskID,
				"status":  "completed",
				"summary": fmt.Sprintf("agent %d completed task", id),
			}
			msg.Context = &MsgContext{TaskID: taskID}

			if err := WriteResult(outboxDir, agentName, msg); err != nil {
				t.Errorf("writer %d: WriteResult failed: %v", id, err)
			}
		}(i)
	}

	for i := 0; i < numScanners; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			// ScanOutbox는 에러 없이 동작해야 한다
			// (쓰기 타이밍에 따라 결과 수는 달라질 수 있음)
			_, err := ScanOutbox(outboxDir)
			if err != nil {
				t.Errorf("scanner %d: ScanOutbox failed: %v", id, err)
			}
		}(i)
	}

	wg.Wait()

	// 최종적으로 모든 쓰기가 완료된 후 스캔 확인
	results, err := ScanOutbox(outboxDir)
	if err != nil {
		t.Fatalf("final ScanOutbox failed: %v", err)
	}
	if len(results) != numWriters {
		t.Errorf("expected %d result files, got %d", numWriters, len(results))
	}

	// 각 결과 파일이 유효한 JSON인지 확인
	for _, path := range results {
		env, err := ReadResult(path)
		if err != nil {
			t.Errorf("ReadResult failed for %s: %v", path, err)
			continue
		}
		if env.Type != MsgResult {
			t.Errorf("file %s: expected type %q, got %q", path, MsgResult, env.Type)
		}
	}
}
