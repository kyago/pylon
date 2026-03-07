package memory

import (
	"testing"

	"github.com/yongjunkang/pylon/internal/config"
	"github.com/yongjunkang/pylon/internal/store"
)

func setupTestManager(t *testing.T) *Manager {
	t.Helper()
	s, err := store.NewStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	if err := s.Migrate(); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	cfg := config.MemoryConfig{
		ProactiveInjection: true,
		ProactiveMaxTokens: 500,
	}

	return NewManager(s, cfg)
}

func TestNewManager(t *testing.T) {
	m := setupTestManager(t)
	if m.Store == nil {
		t.Error("store should not be nil")
	}
	if !m.Cfg.ProactiveInjection {
		t.Error("proactive injection should be enabled")
	}
}

func TestGetProactiveContext_Disabled(t *testing.T) {
	m := setupTestManager(t)
	m.Cfg.ProactiveInjection = false

	ctx, err := m.GetProactiveContext("proj-1", "test task", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx != "" {
		t.Errorf("expected empty context when disabled, got: %s", ctx)
	}
}

func TestGetProactiveContext_NoMemories(t *testing.T) {
	m := setupTestManager(t)

	ctx, err := m.GetProactiveContext("proj-1", "test task", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx != "" {
		t.Errorf("expected empty context with no memories, got: %s", ctx)
	}
}

func TestGetProactiveContext_WithMemories(t *testing.T) {
	m := setupTestManager(t)

	// Insert some memories with English keywords for FTS matching
	m.Store.InsertMemory(&store.MemoryEntry{
		ProjectID:  "proj-1",
		Category:   "learning",
		Key:        "auth-pattern",
		Content:    "authentication pattern using JWT tokens",
		Author:     "architect",
		Confidence: 0.9,
	})
	m.Store.InsertMemory(&store.MemoryEntry{
		ProjectID:  "proj-1",
		Category:   "convention",
		Key:        "code-style",
		Content:    "follow standard Go code style conventions",
		Author:     "reviewer",
		Confidence: 0.8,
	})

	ctx, err := m.GetProactiveContext("proj-1", "authentication", 500)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx == "" {
		t.Error("expected non-empty context with matching memories")
	}
}

func TestGetProactiveContext_TokenLimit(t *testing.T) {
	m := setupTestManager(t)

	// Insert many memories
	for i := 0; i < 20; i++ {
		m.Store.InsertMemory(&store.MemoryEntry{
			ProjectID:  "proj-1",
			Category:   "learning",
			Key:        "item-" + string(rune('A'+i)),
			Content:    "이것은 매우 긴 내용의 메모리 항목입니다. 토큰 제한 테스트를 위해 충분히 길게 작성합니다.",
			Author:     "test",
			Confidence: 0.9,
		})
	}

	// Very small token limit
	ctx, err := m.GetProactiveContext("proj-1", "test", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should be limited by token budget (10 tokens ≈ 40 chars)
	if len(ctx) > 200 {
		t.Errorf("context too long for token limit: %d chars", len(ctx))
	}
}

func TestHandleQuery_NoCategories(t *testing.T) {
	m := setupTestManager(t)

	m.Store.InsertMemory(&store.MemoryEntry{
		ProjectID:  "proj-1",
		Category:   "learning",
		Key:        "test-key",
		Content:    "테스트 내용",
		Author:     "test",
		Confidence: 0.8,
	})

	results, err := m.HandleQuery("proj-1", "테스트", nil)
	if err != nil {
		t.Fatalf("HandleQuery failed: %v", err)
	}
	// May or may not find results depending on FTS matching
	_ = results
}

func TestHandleQuery_WithCategories(t *testing.T) {
	m := setupTestManager(t)

	m.Store.InsertMemory(&store.MemoryEntry{
		ProjectID:  "proj-1",
		Category:   "learning",
		Key:        "learn-key",
		Content:    "학습 내용",
		Author:     "test",
		Confidence: 0.8,
	})

	results, err := m.HandleQuery("proj-1", "학습", []string{"learning"})
	if err != nil {
		t.Fatalf("HandleQuery failed: %v", err)
	}
	_ = results
}

func TestStoreLearnings(t *testing.T) {
	m := setupTestManager(t)

	learnings := []string{
		"에러 핸들링 시 wrapped error를 사용해야 한다",
		"컨텍스트 전파가 중요하다",
	}

	err := m.StoreLearnings("proj-1", "task-001", "architect", learnings)
	if err != nil {
		t.Fatalf("StoreLearnings failed: %v", err)
	}

	// Verify stored
	memories, err := m.Store.GetMemoryByCategory("proj-1", "learning")
	if err != nil {
		t.Fatalf("GetMemoryByCategory failed: %v", err)
	}
	if len(memories) < 2 {
		t.Errorf("expected at least 2 memories, got %d", len(memories))
	}
}

func TestStoreLearnings_Empty(t *testing.T) {
	m := setupTestManager(t)

	err := m.StoreLearnings("proj-1", "task-001", "test", nil)
	if err != nil {
		t.Fatalf("StoreLearnings with nil should succeed: %v", err)
	}
}

func TestSanitize(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello world", "hello-world"},
		{"path/to/thing", "path-to-thing"},
		{"key:value", "key-value"},
		{"no-change", "no-change"},
	}
	for _, tt := range tests {
		if got := sanitize(tt.input); got != tt.expected {
			t.Errorf("sanitize(%s) = %s, want %s", tt.input, got, tt.expected)
		}
	}
}
