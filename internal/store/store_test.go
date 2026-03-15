package store

import (
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
