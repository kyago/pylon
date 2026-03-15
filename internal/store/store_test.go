package store

import (
	"io/fs"
	"sort"
	"strings"
	"testing"
	"time"
)

func setupTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	if err := s.Migrate(); err != nil {
		t.Fatalf("migration failed: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestNewStore_InMemory(t *testing.T) {
	s := setupTestStore(t)
	if s.DB() == nil {
		t.Error("expected non-nil db")
	}
}

func TestMigrate_Idempotent(t *testing.T) {
	s := setupTestStore(t)
	// Running migrate again should not error (IF NOT EXISTS)
	if err := s.Migrate(); err != nil {
		t.Errorf("second migration failed: %v", err)
	}
}

func TestMigrate_FileNameOrder(t *testing.T) {
	// migrations/ 디렉토리의 .sql 파일이 파일명 기준으로 정렬되어 실행되는지 확인
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		t.Fatalf("failed to read migrations dir: %v", err)
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}

	sorted := make([]string, len(names))
	copy(sorted, names)
	sort.Strings(sorted)

	for i, name := range names {
		if name != sorted[i] {
			t.Errorf("migration file order mismatch at index %d: got %q, want %q", i, name, sorted[i])
		}
	}

	// 실제 마이그레이션 실행 후 schema_migrations의 순서 확인
	s := setupTestStore(t)
	rows, err := s.DB().Query(`SELECT version FROM schema_migrations ORDER BY rowid`)
	if err != nil {
		t.Fatalf("failed to query schema_migrations: %v", err)
	}
	defer rows.Close()

	var applied []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		applied = append(applied, v)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}

	if len(applied) != len(sorted) {
		t.Fatalf("expected %d applied migrations, got %d", len(sorted), len(applied))
	}
	for i, v := range applied {
		if v != sorted[i] {
			t.Errorf("applied migration order mismatch at index %d: got %q, want %q", i, v, sorted[i])
		}
	}
}

func TestMigrate_SkipsAlreadyApplied(t *testing.T) {
	s, err := NewStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	// 첫 번째 마이그레이션 실행
	if err := s.Migrate(); err != nil {
		t.Fatalf("first migration failed: %v", err)
	}

	// 실행된 마이그레이션 수 확인
	var countBefore int
	if err := s.DB().QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&countBefore); err != nil {
		t.Fatalf("failed to count migrations: %v", err)
	}

	// 두 번째 마이그레이션 실행 — 이미 적용된 것은 건너뛰어야 함
	if err := s.Migrate(); err != nil {
		t.Fatalf("second migration failed: %v", err)
	}

	var countAfter int
	if err := s.DB().QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&countAfter); err != nil {
		t.Fatalf("failed to count migrations: %v", err)
	}

	if countBefore != countAfter {
		t.Errorf("expected same migration count after re-run: before=%d, after=%d", countBefore, countAfter)
	}
}

func TestMigrate_RecordsAppliedVersions(t *testing.T) {
	s := setupTestStore(t)

	// schema_migrations 테이블에 실행 기록이 있는지 확인
	rows, err := s.DB().Query(`SELECT version, applied_at FROM schema_migrations ORDER BY version`)
	if err != nil {
		t.Fatalf("failed to query schema_migrations: %v", err)
	}
	defer rows.Close()

	var versions []string
	for rows.Next() {
		var version string
		var appliedAt string
		if err := rows.Scan(&version, &appliedAt); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		if version == "" {
			t.Error("version should not be empty")
		}
		if appliedAt == "" {
			t.Error("applied_at should not be empty")
		}
		versions = append(versions, version)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}

	if len(versions) == 0 {
		t.Error("expected at least one migration record in schema_migrations")
	}

	// 모든 .sql 파일이 기록되어 있는지 확인
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		t.Fatalf("failed to read migrations dir: %v", err)
	}

	var expectedFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			expectedFiles = append(expectedFiles, e.Name())
		}
	}

	if len(versions) != len(expectedFiles) {
		t.Errorf("expected %d migration records, got %d", len(expectedFiles), len(versions))
	}
}

// --- Message Queue Tests ---

func TestMessageQueue_EnqueueDequeue(t *testing.T) {
	s := setupTestStore(t)

	msg := &QueuedMessage{
		Type:      "task_assign",
		Priority:  2,
		FromAgent: "orchestrator",
		ToAgent:   "backend-dev",
		Subject:   "Implement login",
		Body:      `{"task_id":"t1","description":"login"}`,
	}

	if err := s.Enqueue(msg); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}
	if msg.ID == "" {
		t.Error("expected auto-generated ID")
	}

	dequeued, err := s.Dequeue("backend-dev")
	if err != nil {
		t.Fatalf("dequeue failed: %v", err)
	}
	if dequeued == nil {
		t.Fatal("expected a message")
	}
	if dequeued.Subject != "Implement login" {
		t.Errorf("expected subject 'Implement login', got %q", dequeued.Subject)
	}
	if dequeued.Status != "delivered" {
		t.Errorf("expected status 'delivered', got %q", dequeued.Status)
	}
}

func TestMessageQueue_DequeueEmpty(t *testing.T) {
	s := setupTestStore(t)

	msg, err := s.Dequeue("nobody")
	if err != nil {
		t.Fatalf("dequeue failed: %v", err)
	}
	if msg != nil {
		t.Error("expected nil for empty queue")
	}
}

func TestMessageQueue_Ack(t *testing.T) {
	s := setupTestStore(t)

	msg := &QueuedMessage{
		Type:      "result",
		FromAgent: "backend-dev",
		ToAgent:   "orchestrator",
		Body:      `{"status":"completed"}`,
	}
	s.Enqueue(msg)

	if err := s.Ack(msg.ID); err != nil {
		t.Fatalf("ack failed: %v", err)
	}
}

func TestMessageQueue_AckNotFound(t *testing.T) {
	s := setupTestStore(t)

	err := s.Ack("nonexistent-id")
	if err == nil {
		t.Error("expected error for nonexistent message")
	}
}

func TestMessageQueue_PriorityOrder(t *testing.T) {
	s := setupTestStore(t)

	// Enqueue low priority first, then high
	s.Enqueue(&QueuedMessage{Type: "task_assign", Priority: 3, FromAgent: "orc", ToAgent: "dev", Body: `{"n":"low"}`})
	s.Enqueue(&QueuedMessage{Type: "task_assign", Priority: 0, FromAgent: "orc", ToAgent: "dev", Body: `{"n":"critical"}`})

	msg, _ := s.Dequeue("dev")
	if msg == nil {
		t.Fatal("expected a message")
	}
	if msg.Priority != 0 {
		t.Errorf("expected critical priority (0), got %d", msg.Priority)
	}
}

func TestMessageQueue_GetPending(t *testing.T) {
	s := setupTestStore(t)

	s.Enqueue(&QueuedMessage{Type: "task_assign", FromAgent: "orc", ToAgent: "dev", Body: `{}`})
	s.Enqueue(&QueuedMessage{Type: "task_assign", FromAgent: "orc", ToAgent: "dev", Body: `{}`})

	pending, err := s.GetPending("dev")
	if err != nil {
		t.Fatalf("GetPending failed: %v", err)
	}
	if len(pending) != 2 {
		t.Errorf("expected 2 pending, got %d", len(pending))
	}
}

// --- GetByTaskID Tests ---

func TestMessageQueue_GetByTaskID_BodyMatch(t *testing.T) {
	s := setupTestStore(t)

	s.Enqueue(&QueuedMessage{
		Type: "task_assign", FromAgent: "orc", ToAgent: "dev",
		Body: `{"task_id":"task-abc","description":"login"}`,
	})
	s.Enqueue(&QueuedMessage{
		Type: "result", FromAgent: "dev", ToAgent: "orc",
		Body: `{"task_id":"task-abc","status":"completed"}`,
	})
	s.Enqueue(&QueuedMessage{
		Type: "task_assign", FromAgent: "orc", ToAgent: "dev",
		Body: `{"task_id":"task-other","description":"signup"}`,
	})

	msgs, err := s.GetByTaskID("task-abc")
	if err != nil {
		t.Fatalf("GetByTaskID failed: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages for task-abc, got %d", len(msgs))
	}
}

func TestMessageQueue_GetByTaskID_ContextMatch(t *testing.T) {
	s := setupTestStore(t)

	s.Enqueue(&QueuedMessage{
		Type: "result", FromAgent: "dev", ToAgent: "orc",
		Body:    `{"status":"completed"}`,
		Context: `{"task_id":"task-ctx-1","pipeline_id":"pipe-1"}`,
	})

	msgs, err := s.GetByTaskID("task-ctx-1")
	if err != nil {
		t.Fatalf("GetByTaskID failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Errorf("expected 1 message for task-ctx-1, got %d", len(msgs))
	}
}

func TestMessageQueue_GetByTaskID_BothBodyAndContext(t *testing.T) {
	s := setupTestStore(t)

	// task_id in body
	s.Enqueue(&QueuedMessage{
		Type: "task_assign", FromAgent: "orc", ToAgent: "dev",
		Body: `{"task_id":"task-both","description":"test"}`,
	})
	// task_id in context only
	s.Enqueue(&QueuedMessage{
		Type: "result", FromAgent: "dev", ToAgent: "orc",
		Body:    `{"status":"completed"}`,
		Context: `{"task_id":"task-both"}`,
	})
	// task_id in both body and context (should not duplicate)
	s.Enqueue(&QueuedMessage{
		Type: "result", FromAgent: "dev", ToAgent: "orc",
		Body:    `{"task_id":"task-both","status":"done"}`,
		Context: `{"task_id":"task-both"}`,
	})

	msgs, err := s.GetByTaskID("task-both")
	if err != nil {
		t.Fatalf("GetByTaskID failed: %v", err)
	}
	if len(msgs) != 3 {
		t.Errorf("expected 3 messages for task-both, got %d", len(msgs))
	}
}

func TestMessageQueue_GetByTaskID_NotFound(t *testing.T) {
	s := setupTestStore(t)

	s.Enqueue(&QueuedMessage{
		Type: "task_assign", FromAgent: "orc", ToAgent: "dev",
		Body: `{"task_id":"task-exists","description":"test"}`,
	})

	msgs, err := s.GetByTaskID("nonexistent-task")
	if err != nil {
		t.Fatalf("GetByTaskID failed: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages for nonexistent task, got %d", len(msgs))
	}
}

func TestMessageQueue_GetByTaskID_SQLInjection(t *testing.T) {
	s := setupTestStore(t)

	s.Enqueue(&QueuedMessage{
		Type: "task_assign", FromAgent: "orc", ToAgent: "dev",
		Body: `{"task_id":"safe-task","description":"test"}`,
	})

	// SQL 인젝션 시도 — 결과가 없어야 하고 에러도 없어야 함
	injectionPayloads := []string{
		`"; DROP TABLE message_queue; --`,
		`' OR '1'='1`,
		`" OR "1"="1`,
		`%`,
		`%%`,
		`' UNION SELECT * FROM message_queue --`,
		`task_id" OR 1=1 --`,
	}

	for _, payload := range injectionPayloads {
		msgs, err := s.GetByTaskID(payload)
		if err != nil {
			t.Errorf("GetByTaskID with injection payload %q returned error: %v", payload, err)
			continue
		}
		if len(msgs) != 0 {
			t.Errorf("GetByTaskID with injection payload %q returned %d messages, expected 0", payload, len(msgs))
		}
	}

	// 원본 데이터가 손상되지 않았는지 확인
	msgs, err := s.GetByTaskID("safe-task")
	if err != nil {
		t.Fatalf("GetByTaskID after injection attempts failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Errorf("expected original data intact, got %d messages", len(msgs))
	}
}

// --- Pipeline State Tests ---

func TestPipelineState_UpsertGet(t *testing.T) {
	s := setupTestStore(t)

	rec := &PipelineRecord{
		PipelineID: "pipe-1",
		Stage:      "init",
		StateJSON:  `{"pipeline_id":"pipe-1"}`,
		UpdatedAt:  time.Now(),
	}

	if err := s.UpsertPipeline(rec); err != nil {
		t.Fatalf("upsert failed: %v", err)
	}

	got, err := s.GetPipeline("pipe-1")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected pipeline record")
	}
	if got.Stage != "init" {
		t.Errorf("expected stage 'init', got %q", got.Stage)
	}
}

func TestPipelineState_Update(t *testing.T) {
	s := setupTestStore(t)

	rec := &PipelineRecord{PipelineID: "pipe-1", Stage: "init", StateJSON: `{}`, UpdatedAt: time.Now()}
	s.UpsertPipeline(rec)

	rec.Stage = "po_conversation"
	rec.UpdatedAt = time.Now()
	s.UpsertPipeline(rec)

	got, _ := s.GetPipeline("pipe-1")
	if got.Stage != "po_conversation" {
		t.Errorf("expected updated stage, got %q", got.Stage)
	}
}

func TestPipelineState_GetActive(t *testing.T) {
	s := setupTestStore(t)

	s.UpsertPipeline(&PipelineRecord{PipelineID: "active-1", Stage: "agent_executing", StateJSON: `{}`, UpdatedAt: time.Now()})
	s.UpsertPipeline(&PipelineRecord{PipelineID: "done-1", Stage: "completed", StateJSON: `{}`, UpdatedAt: time.Now()})

	active, err := s.GetActivePipelines()
	if err != nil {
		t.Fatalf("GetActivePipelines failed: %v", err)
	}
	if len(active) != 1 {
		t.Errorf("expected 1 active pipeline, got %d", len(active))
	}
}

// --- Blackboard Tests ---

func TestBlackboard_PutGet(t *testing.T) {
	s := setupTestStore(t)

	entry := &BlackboardEntry{
		ProjectID:  "proj-1",
		Category:   "decision",
		Key:        "auth-method",
		Value:      `"JWT"`,
		Confidence: 0.9,
		Author:     "architect",
	}

	if err := s.PutBlackboard(entry); err != nil {
		t.Fatalf("put failed: %v", err)
	}

	got, err := s.GetBlackboard("proj-1", "decision", "auth-method")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected entry")
	}
	if got.Value != `"JWT"` {
		t.Errorf("expected value '\"JWT\"', got %q", got.Value)
	}
}

func TestBlackboard_Upsert(t *testing.T) {
	s := setupTestStore(t)

	s.PutBlackboard(&BlackboardEntry{ProjectID: "p1", Category: "decision", Key: "k1", Value: `"v1"`, Confidence: 0.5, Author: "a1"})
	s.PutBlackboard(&BlackboardEntry{ProjectID: "p1", Category: "decision", Key: "k1", Value: `"v2"`, Confidence: 0.9, Author: "a2"})

	got, _ := s.GetBlackboard("p1", "decision", "k1")
	if got.Value != `"v2"` {
		t.Errorf("expected updated value, got %q", got.Value)
	}
}

func TestBlackboard_GetByCategory(t *testing.T) {
	s := setupTestStore(t)

	s.PutBlackboard(&BlackboardEntry{ProjectID: "p1", Category: "decision", Key: "k1", Value: `"v1"`, Author: "a1"})
	s.PutBlackboard(&BlackboardEntry{ProjectID: "p1", Category: "decision", Key: "k2", Value: `"v2"`, Author: "a1"})
	s.PutBlackboard(&BlackboardEntry{ProjectID: "p1", Category: "evidence", Key: "k3", Value: `"v3"`, Author: "a1"})

	entries, err := s.GetBlackboardByCategory("p1", "decision")
	if err != nil {
		t.Fatalf("GetByCategory failed: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 decisions, got %d", len(entries))
	}
}

// --- Project Memory Tests ---

func TestProjectMemory_InsertSearch(t *testing.T) {
	s := setupTestStore(t)

	entry := &MemoryEntry{
		ProjectID:  "proj-1",
		Category:   "learning",
		Key:        "sqlc-nullable",
		Content:    "sqlc에서 nullable 필드는 sql.NullString 사용 필요",
		Confidence: 0.9,
		Author:     "backend-dev",
	}

	if err := s.InsertMemory(entry); err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	results, err := s.SearchMemory("proj-1", "nullable", 10)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 search result, got %d", len(results))
	}
}

func TestProjectMemory_GetByCategory(t *testing.T) {
	s := setupTestStore(t)

	s.InsertMemory(&MemoryEntry{ProjectID: "p1", Category: "pattern", Key: "k1", Content: "c1"})
	s.InsertMemory(&MemoryEntry{ProjectID: "p1", Category: "pattern", Key: "k2", Content: "c2"})
	s.InsertMemory(&MemoryEntry{ProjectID: "p1", Category: "learning", Key: "k3", Content: "c3"})

	entries, err := s.GetMemoryByCategory("p1", "pattern")
	if err != nil {
		t.Fatalf("GetByCategory failed: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 patterns, got %d", len(entries))
	}
}

func TestProjectMemory_IncrementAccess(t *testing.T) {
	s := setupTestStore(t)

	entry := &MemoryEntry{ProjectID: "p1", Category: "learning", Key: "k1", Content: "c1"}
	s.InsertMemory(entry)

	s.IncrementAccessCount(entry.ID)
	s.IncrementAccessCount(entry.ID)

	entries, _ := s.GetMemoryByCategory("p1", "learning")
	if len(entries) > 0 && entries[0].AccessCount != 2 {
		t.Errorf("expected access count 2, got %d", entries[0].AccessCount)
	}
}

func TestProjectMemory_FTSTrigger_UpdateSync(t *testing.T) {
	s := setupTestStore(t)

	entry := &MemoryEntry{
		ProjectID:  "proj-1",
		Category:   "learning",
		Key:        "original-key",
		Content:    "original content about authentication",
		Confidence: 0.8,
		Author:     "backend-dev",
	}
	if err := s.InsertMemory(entry); err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	// UPDATE를 직접 실행하여 트리거 동작을 검증
	_, err := s.DB().Exec(
		`UPDATE project_memory SET content = ?, key = ? WHERE id = ?`,
		"updated content about authorization", "updated-key", entry.ID,
	)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	// 기존 검색어로는 결과 없어야 함
	results, err := s.SearchMemory("proj-1", "authentication", 10)
	if err != nil {
		t.Fatalf("search old term failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for old content, got %d", len(results))
	}

	// 새 검색어로는 결과 있어야 함
	results, err = s.SearchMemory("proj-1", "authorization", 10)
	if err != nil {
		t.Fatalf("search new term failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result for updated content, got %d", len(results))
	}
}

func TestProjectMemory_FTSTrigger_DeleteSync(t *testing.T) {
	s := setupTestStore(t)

	entry := &MemoryEntry{
		ProjectID:  "proj-1",
		Category:   "learning",
		Key:        "delete-test",
		Content:    "content to be deleted about middleware",
		Confidence: 0.8,
		Author:     "backend-dev",
	}
	if err := s.InsertMemory(entry); err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	// 삭제 전 검색 가능 확인
	results, err := s.SearchMemory("proj-1", "middleware", 10)
	if err != nil {
		t.Fatalf("search before delete failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result before delete, got %d", len(results))
	}

	// DELETE를 직접 실행하여 트리거 동작을 검증
	_, err = s.DB().Exec(`DELETE FROM project_memory WHERE id = ?`, entry.ID)
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	// 삭제 후 검색 결과 없어야 함
	results, err = s.SearchMemory("proj-1", "middleware", 10)
	if err != nil {
		t.Fatalf("search after delete failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results after delete, got %d", len(results))
	}
}

// --- FTS5 Query Sanitization Tests ---

func TestSanitizeFTS5Query(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"정상 단어", "nullable", `"nullable"`},
		{"여러 단어", "sql nullable field", `"sql" "nullable" "field"`},
		{"AND 연산자 제거", "sql AND nullable", `"sql" "nullable"`},
		{"OR 연산자 제거", "sql OR nullable", `"sql" "nullable"`},
		{"NOT 연산자 제거", "NOT nullable", `"nullable"`},
		{"NEAR 연산자 제거", "NEAR nullable", `"nullable"`},
		{"소문자 연산자 제거", "sql and nullable", `"sql" "nullable"`},
		{"따옴표 제거", `"unclosed`, `"unclosed"`},
		{"짝 안맞는 따옴표", `"hello world`, `"hello" "world"`},
		{"별표 제거", "test*", `"test"`},
		{"괄호 제거", "(test)", `"test"`},
		{"빈 문자열", "", ""},
		{"공백만", "   ", ""},
		{"특수문자만", "* ^ : + -", ""},
		{"연산자만", "AND OR NOT", ""},
		{"혼합 쿼리", `"hello AND world*`, `"hello" "world"`},
		{"한글 쿼리", "인증 시스템", `"인증" "시스템"`},
		{"콜론 포함", "category:learning", `"category" "learning"`},
		{"중괄호 제거", "{test}", `"test"`},
		{"하이픈 포함 키 유지", "sqlc-nullable", `"sqlc-nullable"`},
		{"하이픈 여러 개 유지", "my-long-key-name", `"my-long-key-name"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeFTS5Query(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeFTS5Query(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSearchMemory_EmptyQuery(t *testing.T) {
	s := setupTestStore(t)

	entry := &MemoryEntry{
		ProjectID:  "proj-1",
		Category:   "learning",
		Key:        "test-key",
		Content:    "test content",
		Confidence: 0.9,
	}
	if err := s.InsertMemory(entry); err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	// 빈 쿼리 — nil 반환
	results, err := s.SearchMemory("proj-1", "", 10)
	if err != nil {
		t.Fatalf("search with empty query failed: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil for empty query, got %d results", len(results))
	}
}

func TestSearchMemory_SpecialCharactersOnly(t *testing.T) {
	s := setupTestStore(t)

	entry := &MemoryEntry{
		ProjectID:  "proj-1",
		Category:   "learning",
		Key:        "test-key",
		Content:    "test content",
		Confidence: 0.9,
	}
	if err := s.InsertMemory(entry); err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	// 특수문자만 있는 쿼리 — nil 반환
	results, err := s.SearchMemory("proj-1", "***", 10)
	if err != nil {
		t.Fatalf("search with special chars only failed: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil for special chars only query, got %d results", len(results))
	}
}

func TestSearchMemory_FTS5SpecialCharsNoError(t *testing.T) {
	s := setupTestStore(t)

	entry := &MemoryEntry{
		ProjectID:  "proj-1",
		Category:   "learning",
		Key:        "sqlc-nullable",
		Content:    "sqlc에서 nullable 필드는 sql.NullString 사용 필요",
		Confidence: 0.9,
		Author:     "backend-dev",
	}
	if err := s.InsertMemory(entry); err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	// FTS5 문법 에러를 유발할 수 있는 쿼리들이 에러 없이 동작해야 함
	dangerousQueries := []string{
		`"unclosed`,
		`nullable AND`,
		`OR OR OR`,
		`***`,
		`"hello" OR "world`,
		`NEAR(nullable, 5)`,
		`nullable*`,
		`^nullable`,
		`column:nullable`,
		`{nullable}`,
		`(nullable OR)`,
		`NOT`,
		`"`,
		`""`,
	}

	for _, q := range dangerousQueries {
		results, err := s.SearchMemory("proj-1", q, 10)
		if err != nil {
			t.Errorf("SearchMemory(%q) returned error: %v", q, err)
		}
		// 에러만 안 나면 OK — 결과 유무는 쿼리에 따라 다름
		_ = results
	}
}

func TestSearchMemory_NormalQueryStillWorks(t *testing.T) {
	s := setupTestStore(t)

	entry := &MemoryEntry{
		ProjectID:  "proj-1",
		Category:   "learning",
		Key:        "sqlc-nullable",
		Content:    "sqlc에서 nullable 필드는 sql.NullString 사용 필요",
		Confidence: 0.9,
		Author:     "backend-dev",
	}
	if err := s.InsertMemory(entry); err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	// sanitization 적용 후에도 정상 쿼리가 제대로 동작해야 함
	results, err := s.SearchMemory("proj-1", "nullable", 10)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestSearchMemory_HyphenatedKeyMatch(t *testing.T) {
	s := setupTestStore(t)

	entry := &MemoryEntry{
		ProjectID:  "proj-1",
		Category:   "learning",
		Key:        "sqlc-nullable",
		Content:    "sqlc에서 nullable 필드는 sql.NullString 사용 필요",
		Confidence: 0.9,
		Author:     "backend-dev",
	}
	if err := s.InsertMemory(entry); err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	// 하이픈 포함 키로 검색 시 매칭되어야 함
	results, err := s.SearchMemory("proj-1", "sqlc-nullable", 10)
	if err != nil {
		t.Fatalf("search with hyphenated key failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result for hyphenated key search, got %d", len(results))
	}
}

// --- Session Archive Tests ---

func TestSessionArchive_ArchiveAndGet(t *testing.T) {
	s := setupTestStore(t)

	entry := &ArchiveEntry{
		AgentName:  "backend-dev",
		TaskID:     "task-1",
		Summary:    "Implemented login API",
		TokenCount: 5000,
	}

	if err := s.Archive(entry); err != nil {
		t.Fatalf("archive failed: %v", err)
	}

	byAgent, err := s.GetArchiveByAgent("backend-dev", 10)
	if err != nil {
		t.Fatalf("GetByAgent failed: %v", err)
	}
	if len(byAgent) != 1 {
		t.Errorf("expected 1 archive, got %d", len(byAgent))
	}

	byTask, err := s.GetArchiveByTask("task-1")
	if err != nil {
		t.Fatalf("GetByTask failed: %v", err)
	}
	if len(byTask) != 1 {
		t.Errorf("expected 1 archive, got %d", len(byTask))
	}
}

func TestSessionArchive_GetRecent(t *testing.T) {
	s := setupTestStore(t)

	s.Archive(&ArchiveEntry{AgentName: "a1", TaskID: "t1", Summary: "s1"})
	s.Archive(&ArchiveEntry{AgentName: "a2", TaskID: "t2", Summary: "s2"})

	recent, err := s.GetRecentArchives(10)
	if err != nil {
		t.Fatalf("GetRecent failed: %v", err)
	}
	if len(recent) != 2 {
		t.Errorf("expected 2 archives, got %d", len(recent))
	}
}
