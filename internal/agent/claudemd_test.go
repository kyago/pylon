package agent

import (
	"strings"
	"testing"
)

// --- CommunicationRulesWithPaths Tests ---

func TestCommunicationRulesWithPaths_InboxPathOnly(t *testing.T) {
	// inboxPath는 함수 시그니처에 존재하지만, 현재 구현에서 outbox 쪽 경로만 분기한다.
	// inboxPath만 전달해도 기본 프로토콜 규칙이 반환되는지 확인한다.
	result := CommunicationRulesWithPaths("/workspace/inbox", "", "")

	if !strings.Contains(result, "파일 기반 메시지 프로토콜") {
		t.Error("기본 프로토콜 헤더가 포함되어야 함")
	}
	if !strings.Contains(result, "inbox에서 태스크 파일을 읽어") {
		t.Error("inbox 읽기 지시가 포함되어야 함")
	}
	// outboxPath가 비어있으므로 기본 템플릿 경로 패턴 사용
	if !strings.Contains(result, "{에이전트명}") {
		t.Error("outboxPath 미지정 시 기본 템플릿 경로가 사용되어야 함")
	}
}

func TestCommunicationRulesWithPaths_OutboxPathOnly(t *testing.T) {
	outbox := "/workspace/.pylon/runtime/outbox/dev/task-001.result.json"
	result := CommunicationRulesWithPaths("", outbox, "")

	if !strings.Contains(result, "파일 기반 메시지 프로토콜") {
		t.Error("기본 프로토콜 헤더가 포함되어야 함")
	}
	// outboxPath가 지정되면 구체적인 경로가 포함되어야 함
	if !strings.Contains(result, outbox) {
		t.Errorf("outboxPath(%s)가 결과에 포함되어야 함", outbox)
	}
	// .tmp.json 임시 파일 경로도 포함되어야 함
	expectedTmp := "/workspace/.pylon/runtime/outbox/dev/task-001.tmp.json"
	if !strings.Contains(result, expectedTmp) {
		t.Errorf("임시 파일 경로(%s)가 포함되어야 함", expectedTmp)
	}
	// 기본 템플릿 패턴은 나타나지 않아야 함
	if strings.Contains(result, "{에이전트명}") {
		t.Error("outboxPath 지정 시 기본 템플릿 패턴이 나타나면 안 됨")
	}
}

func TestCommunicationRulesWithPaths_OutboxDirOnly(t *testing.T) {
	outboxDir := "/workspace/.pylon/runtime/outbox/dev"
	result := CommunicationRulesWithPaths("", "", outboxDir)

	if !strings.Contains(result, "mkdir -p") {
		t.Error("outboxDir 지정 시 mkdir 지시가 포함되어야 함")
	}
	if !strings.Contains(result, outboxDir) {
		t.Errorf("outboxDir 경로(%s)가 결과에 포함되어야 함", outboxDir)
	}
}

func TestCommunicationRulesWithPaths_AllPaths(t *testing.T) {
	inboxPath := "/workspace/.pylon/runtime/inbox"
	outboxPath := "/workspace/.pylon/runtime/outbox/dev/task-001.result.json"
	outboxDir := "/workspace/.pylon/runtime/outbox/dev"

	result := CommunicationRulesWithPaths(inboxPath, outboxPath, outboxDir)

	// outboxPath의 구체적 경로
	if !strings.Contains(result, outboxPath) {
		t.Error("outboxPath가 결과에 포함되어야 함")
	}
	// 임시 파일 경로
	if !strings.Contains(result, "task-001.tmp.json") {
		t.Error("임시 파일 경로가 포함되어야 함")
	}
	// mkdir 지시
	if !strings.Contains(result, "mkdir -p") {
		t.Error("outboxDir 지정 시 mkdir 지시가 포함되어야 함")
	}
	if !strings.Contains(result, outboxDir) {
		t.Error("outboxDir 경로가 결과에 포함되어야 함")
	}
	// JSON 형식 안내
	if !strings.Contains(result, "결과 파일 JSON 형식") {
		t.Error("JSON 형식 안내가 포함되어야 함")
	}
}

func TestCommunicationRulesWithPaths_AllEmpty(t *testing.T) {
	result := CommunicationRulesWithPaths("", "", "")
	defaultResult := DefaultCommunicationRules()

	if result != defaultResult {
		t.Error("모든 경로가 빈 문자열이면 DefaultCommunicationRules()와 동일해야 함")
	}
}

// --- DefaultCompactionRules Tests ---

func TestDefaultCompactionRules_NotEmpty(t *testing.T) {
	result := DefaultCompactionRules()

	if result == "" {
		t.Error("DefaultCompactionRules()는 빈 문자열이 아니어야 함")
	}
}

func TestDefaultCompactionRules_ContainsKeywords(t *testing.T) {
	result := DefaultCompactionRules()

	keywords := []string{
		"컨텍스트 관리",
		"context",
		"summary",
		"decisions",
		"references",
	}

	for _, kw := range keywords {
		if !strings.Contains(result, kw) {
			t.Errorf("DefaultCompactionRules에 키워드 %q가 포함되어야 함", kw)
		}
	}
}

// --- buildDomainSection Tests ---

func TestBuildDomainSection_EmptyPaths(t *testing.T) {
	result := buildDomainSection(nil)
	if result != "" {
		t.Errorf("빈 paths는 빈 문자열을 반환해야 함, got %q", result)
	}

	result = buildDomainSection([]string{})
	if result != "" {
		t.Errorf("빈 슬라이스도 빈 문자열을 반환해야 함, got %q", result)
	}
}

func TestBuildDomainSection_SinglePath(t *testing.T) {
	result := buildDomainSection([]string{".pylon/domain/architecture.md"})

	if !strings.Contains(result, "참조할 도메인 지식 문서:") {
		t.Error("헤더가 포함되어야 함")
	}
	if !strings.Contains(result, "- .pylon/domain/architecture.md") {
		t.Error("경로가 리스트 아이템으로 포함되어야 함")
	}
	if !strings.Contains(result, "필요한 경우 위 파일을 직접 읽어 참고하세요.") {
		t.Error("안내 문구가 포함되어야 함")
	}
}

func TestBuildDomainSection_MultiplePaths(t *testing.T) {
	paths := []string{
		".pylon/domain/architecture.md",
		".pylon/domain/conventions.md",
		".pylon/domain/api-spec.md",
	}
	result := buildDomainSection(paths)

	for _, p := range paths {
		if !strings.Contains(result, "- "+p) {
			t.Errorf("경로 %q가 결과에 포함되어야 함", p)
		}
	}

	// 경로 개수만큼 "- " 패턴이 있어야 함
	count := strings.Count(result, "- ")
	if count != len(paths) {
		t.Errorf("리스트 아이템 수 = %d, want %d", count, len(paths))
	}
}

// --- Build 통합 테스트: CommunicationRulesWithPaths 결과 사용 ---

func TestClaudeMDBuilder_BuildWithCommunicationRulesWithPaths(t *testing.T) {
	builder := &ClaudeMDBuilder{MaxLines: 200}

	commRules := CommunicationRulesWithPaths(
		"/workspace/.pylon/runtime/inbox",
		"/workspace/.pylon/runtime/outbox/dev/task-001.result.json",
		"/workspace/.pylon/runtime/outbox/dev",
	)

	result, err := builder.Build(BuildInput{
		CommunicationRules: commRules,
		TaskContext:        "백엔드 API 개발\n수용 기준: POST /api/users",
		CompactionRules:    DefaultCompactionRules(),
		ProjectMemory:      "Echo 프레임워크 사용",
		DomainPaths:        []string{".pylon/domain/architecture.md"},
	})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	lines := strings.Split(result, "\n")
	if len(lines) > 200 {
		t.Errorf("200줄 제한 초과: %d줄", len(lines))
	}

	// Communication Rules 섹션에 구체적 outbox 경로가 포함되어야 함
	if !strings.Contains(result, "## Communication Rules") {
		t.Error("Communication Rules 섹션 헤더가 없음")
	}
	if !strings.Contains(result, "task-001.result.json") {
		t.Error("outbox 구체적 경로가 Build 결과에 포함되어야 함")
	}
	if !strings.Contains(result, "mkdir -p") {
		t.Error("outboxDir mkdir 지시가 Build 결과에 포함되어야 함")
	}

	// 다른 섹션도 정상 포함되어야 함
	if !strings.Contains(result, "## Task Context") {
		t.Error("Task Context 섹션이 없음")
	}
	if !strings.Contains(result, "## Context Management") {
		t.Error("Context Management 섹션이 없음")
	}
	if !strings.Contains(result, "## Domain Knowledge") {
		t.Error("Domain Knowledge 섹션이 없음")
	}
}
