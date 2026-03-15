package store

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// setupConcurrentTestStore creates a file-based SQLite store for concurrency
// tests. In-memory ":memory:" databases create separate instances per
// connection in the sql.DB pool, which causes "no such table" errors under
// concurrent access. A file-based store with WAL mode and busy_timeout
// properly supports concurrent reads/writes.
func setupConcurrentTestStore(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")

	// busy_timeout을 DSN에 설정하여 모든 커넥션에 적용
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	s := &Store{db: db}
	if err := s.Migrate(); err != nil {
		t.Fatalf("migration failed: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// TestMessageQueue_ConcurrentEnqueue verifies that 10 goroutines can
// simultaneously enqueue messages without data loss or race conditions.
func TestMessageQueue_ConcurrentEnqueue(t *testing.T) {
	s := setupConcurrentTestStore(t)

	const numGoroutines = 10
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			msg := &QueuedMessage{
				Type:      "task_assign",
				FromAgent: "orchestrator",
				ToAgent:   fmt.Sprintf("agent-%d", id),
				Body:      fmt.Sprintf(`{"task": "task-%d"}`, id),
			}
			if err := s.Enqueue(msg); err != nil {
				t.Errorf("goroutine %d: enqueue failed: %v", id, err)
			}
		}(i)
	}

	wg.Wait()

	// 모든 메시지가 삽입되었는지 확인
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM message_queue").Scan(&count)
	if err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	if count != numGoroutines {
		t.Errorf("expected %d messages, got %d", numGoroutines, count)
	}
}

// TestMessageQueue_ConcurrentDequeue verifies that each message is dequeued
// exactly once when multiple goroutines compete for the same queue.
func TestMessageQueue_ConcurrentDequeue(t *testing.T) {
	s := setupConcurrentTestStore(t)

	const numMessages = 20
	const numWorkers = 5

	// 모든 메시지를 동일 에이전트에게 enqueue
	for i := 0; i < numMessages; i++ {
		msg := &QueuedMessage{
			Type:      "task_assign",
			FromAgent: "orchestrator",
			ToAgent:   "shared-agent",
			Body:      fmt.Sprintf(`{"task": "task-%d"}`, i),
		}
		if err := s.Enqueue(msg); err != nil {
			t.Fatalf("enqueue %d failed: %v", i, err)
		}
	}

	// 동시에 dequeue하여 중복이 없는지 확인
	var mu sync.Mutex
	dequeued := make(map[string]bool)
	var wg sync.WaitGroup

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for {
				msg, err := s.Dequeue("shared-agent")
				if err != nil {
					// SQLite는 동시 트랜잭션 승격 시 SQLITE_BUSY가 발생할 수 있음.
					// 실제 환경에서도 재시도가 필요한 정상적인 동작이므로 재시도 수행.
					if strings.Contains(err.Error(), "database is locked") {
						time.Sleep(time.Millisecond)
						continue
					}
					t.Errorf("worker %d: dequeue error: %v", workerID, err)
					return
				}
				if msg == nil {
					// 큐가 비었으면 종료
					return
				}
				mu.Lock()
				if dequeued[msg.ID] {
					t.Errorf("worker %d: message %s dequeued more than once", workerID, msg.ID)
				}
				dequeued[msg.ID] = true
				mu.Unlock()
			}
		}(w)
	}

	wg.Wait()

	if len(dequeued) != numMessages {
		t.Errorf("expected %d unique dequeued messages, got %d", numMessages, len(dequeued))
	}
}

// TestBlackboard_ConcurrentPut verifies that concurrent PutBlackboard calls
// on the same key result in a valid final state without corruption.
func TestBlackboard_ConcurrentPut(t *testing.T) {
	s := setupConcurrentTestStore(t)

	const numGoroutines = 10
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			entry := &BlackboardEntry{
				ProjectID:  "proj-concurrent",
				Category:   "decision",
				Key:        "shared-key",
				Value:      fmt.Sprintf(`"value-%d"`, id),
				Confidence: float64(id) / float64(numGoroutines),
				Author:     fmt.Sprintf("agent-%d", id),
			}
			if err := s.PutBlackboard(entry); err != nil {
				t.Errorf("goroutine %d: PutBlackboard failed: %v", id, err)
			}
		}(i)
	}

	wg.Wait()

	// 최종 상태가 유효한지 확인 (UPSERT이므로 1건만 존재)
	got, err := s.GetBlackboard("proj-concurrent", "decision", "shared-key")
	if err != nil {
		t.Fatalf("GetBlackboard failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil blackboard entry")
	}
	if got.Value == "" {
		t.Error("expected non-empty value")
	}
	if got.Author == "" {
		t.Error("expected non-empty author")
	}
}

// TestProjectMemory_ConcurrentInsertSearch verifies that InsertMemory and
// SearchMemory can run concurrently without errors.
func TestProjectMemory_ConcurrentInsertSearch(t *testing.T) {
	s := setupConcurrentTestStore(t)

	const numInserts = 10
	const numSearches = 5
	var wg sync.WaitGroup

	// 삽입과 검색을 동시에 수행
	for i := 0; i < numInserts; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			entry := &MemoryEntry{
				ProjectID:  "proj-concurrent",
				Category:   "learning",
				Key:        fmt.Sprintf("key-%d", id),
				Content:    fmt.Sprintf("concurrent test content number %d with searchable term", id),
				Confidence: 0.8,
				Author:     fmt.Sprintf("agent-%d", id),
			}
			if err := s.InsertMemory(entry); err != nil {
				t.Errorf("goroutine %d: InsertMemory failed: %v", id, err)
			}
		}(i)
	}

	for i := 0; i < numSearches; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			// 검색은 에러가 없어야 한다 (결과 수는 삽입 타이밍에 따라 달라질 수 있음)
			_, err := s.SearchMemory("proj-concurrent", "searchable", 10)
			if err != nil {
				t.Errorf("goroutine search-%d: SearchMemory failed: %v", id, err)
			}
		}(i)
	}

	wg.Wait()

	// 최종적으로 모든 삽입이 완료된 후 검색 결과 확인
	results, err := s.SearchMemory("proj-concurrent", "searchable", 20)
	if err != nil {
		t.Fatalf("final SearchMemory failed: %v", err)
	}
	if len(results) != numInserts {
		t.Errorf("expected %d search results, got %d", numInserts, len(results))
	}
}
