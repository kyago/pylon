package protocol

import (
	"strings"
	"testing"

	"github.com/yongjunkang/pylon/internal/store"
)

func TestBuildHandoffContext_NilPrevResult(t *testing.T) {
	ctx := BuildHandoffContext(nil, nil, nil)
	if ctx == nil {
		t.Fatal("context should not be nil")
	}
	if ctx.Summary != "" {
		t.Errorf("summary should be empty, got: %s", ctx.Summary)
	}
}

func TestBuildHandoffContext_WithPrevResult(t *testing.T) {
	prev := &MessageEnvelope{
		From:    "architect",
		Subject: "구조 분석 완료",
		Context: &MsgContext{
			TaskID:     "task-001",
			PipelineID: "pipe-001",
			ProjectID:  "proj-001",
		},
		Body: map[string]any{
			"summary": "MVC 패턴 적용 권장",
		},
	}

	ctx := BuildHandoffContext(prev, nil, nil)

	if ctx.TaskID != "task-001" {
		t.Errorf("taskID = %s, want task-001", ctx.TaskID)
	}
	if ctx.PipelineID != "pipe-001" {
		t.Errorf("pipelineID = %s, want pipe-001", ctx.PipelineID)
	}
	if !strings.Contains(ctx.Summary, "architect") {
		t.Errorf("summary should contain agent name: %s", ctx.Summary)
	}
	if !strings.Contains(ctx.Summary, "구조 분석 완료") {
		t.Errorf("summary should contain subject: %s", ctx.Summary)
	}
	if !strings.Contains(ctx.Summary, "MVC 패턴 적용 권장") {
		t.Errorf("summary should contain body summary: %s", ctx.Summary)
	}
}

func TestBuildHandoffContext_WithBlackboard(t *testing.T) {
	blackboard := []store.BlackboardEntry{
		{Category: "decision", Key: "framework", Value: "React", Confidence: 0.95},
		{Category: "decision", Key: "database", Value: "PostgreSQL", Confidence: 0.8},
		{Category: "observation", Key: "traffic", Value: "high"},
	}

	ctx := BuildHandoffContext(nil, blackboard, nil)

	if len(ctx.Decisions) != 2 {
		t.Errorf("expected 2 decisions, got %d", len(ctx.Decisions))
	}
	if !strings.Contains(ctx.Decisions[0], "framework") {
		t.Errorf("first decision should be about framework: %s", ctx.Decisions[0])
	}
	if !strings.Contains(ctx.Decisions[0], "95%") {
		t.Errorf("decision should contain confidence: %s", ctx.Decisions[0])
	}
}

func TestBuildHandoffContext_WithMemories(t *testing.T) {
	memories := []store.MemorySearchResult{
		{MemoryEntry: store.MemoryEntry{Category: "learning", Key: "auth-pattern"}},
		{MemoryEntry: store.MemoryEntry{Category: "convention", Key: "naming"}},
	}

	ctx := BuildHandoffContext(nil, nil, memories)

	if !strings.Contains(ctx.Summary, "관련 프로젝트 메모리") {
		t.Errorf("summary should contain memory references: %s", ctx.Summary)
	}
	if !strings.Contains(ctx.Summary, "auth-pattern") {
		t.Errorf("summary should contain memory key: %s", ctx.Summary)
	}
}

func TestBuildHandoffContext_Full(t *testing.T) {
	prev := &MessageEnvelope{
		From:    "reviewer",
		Subject: "코드 리뷰 완료",
		Context: &MsgContext{TaskID: "t-1"},
		Body:    map[string]any{"summary": "보안 취약점 3건 발견"},
	}

	blackboard := []store.BlackboardEntry{
		{Category: "decision", Key: "fix-priority", Value: "XSS먼저", Confidence: 0.9},
	}

	memories := []store.MemorySearchResult{
		{MemoryEntry: store.MemoryEntry{Category: "security", Key: "xss-pattern"}},
	}

	ctx := BuildHandoffContext(prev, blackboard, memories)

	if ctx.TaskID != "t-1" {
		t.Errorf("taskID = %s, want t-1", ctx.TaskID)
	}
	if len(ctx.Decisions) != 1 {
		t.Errorf("expected 1 decision, got %d", len(ctx.Decisions))
	}
	if !strings.Contains(ctx.Summary, "코드 리뷰 완료") {
		t.Error("summary should contain subject")
	}
	if !strings.Contains(ctx.Summary, "xss-pattern") {
		t.Error("summary should contain memory reference")
	}
}

func TestBuildHandoffContext_PrevResultNoContext(t *testing.T) {
	prev := &MessageEnvelope{
		From:    "agent",
		Subject: "test",
	}

	ctx := BuildHandoffContext(prev, nil, nil)
	if ctx.TaskID != "" {
		t.Errorf("taskID should be empty when prev has no context, got: %s", ctx.TaskID)
	}
	if !strings.Contains(ctx.Summary, "agent") {
		t.Errorf("summary should still contain agent name: %s", ctx.Summary)
	}
}

func TestBuildHandoffContext_NoBlackboardDecisions(t *testing.T) {
	blackboard := []store.BlackboardEntry{
		{Category: "observation", Key: "note", Value: "something"},
	}

	ctx := BuildHandoffContext(nil, blackboard, nil)
	if len(ctx.Decisions) != 0 {
		t.Errorf("expected 0 decisions for non-decision entries, got %d", len(ctx.Decisions))
	}
}
