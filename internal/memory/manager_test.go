package memory

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/store"
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

func TestStoreLearnings_MultibyteKeyNotCorrupted(t *testing.T) {
	m := setupTestManager(t)

	// 한글은 3바이트 문자이므로 50바이트 지점이 문자 중간에 걸린다.
	long := strings.Repeat("가나다라마바사아자차", 4) // 40자, 120바이트
	if err := m.StoreLearnings("proj-1", "task-002", "test", []string{long}); err != nil {
		t.Fatalf("StoreLearnings failed: %v", err)
	}

	memories, err := m.Store.GetMemoryByCategory("proj-1", "learning")
	if err != nil {
		t.Fatalf("GetMemoryByCategory failed: %v", err)
	}
	if len(memories) == 0 {
		t.Fatal("expected stored memory")
	}
	for _, mem := range memories {
		if !utf8.ValidString(mem.Key) {
			t.Errorf("key contains invalid UTF-8: %q", mem.Key)
		}
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
