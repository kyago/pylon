package orchestrator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConversationManager_Create(t *testing.T) {
	dir := t.TempDir()
	mgr := NewConversationManager(dir)

	conv, err := mgr.Create("conv-001", "테스트 대화")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if conv.ID != "conv-001" {
		t.Errorf("ID = %s, want conv-001", conv.ID)
	}
	if conv.Meta.Status != "active" {
		t.Errorf("status = %s, want active", conv.Meta.Status)
	}

	// Check meta.yml exists
	metaPath := filepath.Join(dir, "conv-001", "meta.yml")
	if _, err := os.Stat(metaPath); os.IsNotExist(err) {
		t.Error("meta.yml should exist")
	}

	// Check thread.md exists with header
	threadPath := filepath.Join(dir, "conv-001", "thread.md")
	data, err := os.ReadFile(threadPath)
	if err != nil {
		t.Fatalf("failed to read thread.md: %v", err)
	}
	if !strings.Contains(string(data), "테스트 대화") {
		t.Error("thread.md should contain conversation title")
	}
}

func TestConversationManager_AppendMessage(t *testing.T) {
	dir := t.TempDir()
	mgr := NewConversationManager(dir)

	mgr.Create("conv-002", "메시지 테스트")

	if err := mgr.AppendMessage("conv-002", "user", "안녕하세요"); err != nil {
		t.Fatalf("AppendMessage failed: %v", err)
	}
	if err := mgr.AppendMessage("conv-002", "assistant", "반갑습니다"); err != nil {
		t.Fatalf("AppendMessage failed: %v", err)
	}

	threadPath := filepath.Join(dir, "conv-002", "thread.md")
	data, err := os.ReadFile(threadPath)
	if err != nil {
		t.Fatalf("failed to read thread.md: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "user") {
		t.Error("thread should contain user role")
	}
	if !strings.Contains(content, "안녕하세요") {
		t.Error("thread should contain user message")
	}
	if !strings.Contains(content, "assistant") {
		t.Error("thread should contain assistant role")
	}
	if !strings.Contains(content, "반갑습니다") {
		t.Error("thread should contain assistant message")
	}
}

func TestConversationManager_Load(t *testing.T) {
	dir := t.TempDir()
	mgr := NewConversationManager(dir)

	mgr.Create("conv-003", "로드 테스트")

	conv, err := mgr.Load("conv-003")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if conv.ID != "conv-003" {
		t.Errorf("ID = %s, want conv-003", conv.ID)
	}
	if conv.Meta.Status != "active" {
		t.Errorf("status = %s, want active", conv.Meta.Status)
	}
}

func TestConversationManager_Load_NotFound(t *testing.T) {
	dir := t.TempDir()
	mgr := NewConversationManager(dir)

	_, err := mgr.Load("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent conversation")
	}
}

func TestConversationManager_SaveMeta(t *testing.T) {
	dir := t.TempDir()
	mgr := NewConversationManager(dir)

	mgr.Create("conv-004", "메타 업데이트")

	newMeta := ConversationMeta{
		Status:   "completed",
		TaskID:   "task-123",
		Projects: []string{"proj-a"},
	}
	if err := mgr.SaveMeta("conv-004", newMeta); err != nil {
		t.Fatalf("SaveMeta failed: %v", err)
	}

	conv, err := mgr.Load("conv-004")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if conv.Meta.Status != "completed" {
		t.Errorf("status = %s, want completed", conv.Meta.Status)
	}
	if conv.Meta.TaskID != "task-123" {
		t.Errorf("taskID = %s, want task-123", conv.Meta.TaskID)
	}
}
