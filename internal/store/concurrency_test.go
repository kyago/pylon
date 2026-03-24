package store

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

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
