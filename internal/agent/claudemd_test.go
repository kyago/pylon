package agent

import (
	"strings"
	"testing"
)

// --- CommunicationRulesWithPaths Tests ---

func TestCommunicationRulesWithPaths_NoOutputPath(t *testing.T) {
	result := CommunicationRulesWithPaths("", "", "")

	if !strings.Contains(result, "에이전트 실행 규칙") {
		t.Error("에이전트 실행 규칙 헤더가 포함되어야 함")
	}
	if !strings.Contains(result, "할당된 태스크를 수행합니다") {
		t.Error("태스크 수행 지시가 포함되어야 함")
	}
	if !strings.Contains(result, "파이프라인 런타임 디렉토리에 산출물로 저장") {
		t.Error("outputPath 미지정 시 기본 산출물 저장 지시가 포함되어야 함")
	}
}

func TestCommunicationRulesWithPaths_WithOutputPath(t *testing.T) {
	outputPath := "/workspace/.pylon/runtime/pipeline-001/execution-log.json"
	result := CommunicationRulesWithPaths("", outputPath, "")

	if !strings.Contains(result, outputPath) {
		t.Errorf("outputPath(%s)가 결과에 포함되어야 함", outputPath)
	}
	// 기본 산출물 지시는 나타나지 않아야 함
	if strings.Contains(result, "파이프라인 런타임 디렉토리에 산출물로 저장") {
		t.Error("outputPath 지정 시 기본 산출물 지시가 나타나면 안 됨")
	}
}

func TestCommunicationRulesWithPaths_WithOutputDir(t *testing.T) {
	outputDir := "/workspace/.pylon/runtime/pipeline-001"
	result := CommunicationRulesWithPaths("", "", outputDir)

	if !strings.Contains(result, "mkdir -p") {
		t.Error("outputDir 지정 시 mkdir 지시가 포함되어야 함")
	}
	if !strings.Contains(result, outputDir) {
		t.Errorf("outputDir 경로(%s)가 결과에 포함되어야 함", outputDir)
	}
}

func TestCommunicationRulesWithPaths_AllPaths(t *testing.T) {
	outputPath := "/workspace/.pylon/runtime/pipeline-001/execution-log.json"
	outputDir := "/workspace/.pylon/runtime/pipeline-001"

	result := CommunicationRulesWithPaths("", outputPath, outputDir)

	if !strings.Contains(result, outputPath) {
		t.Error("outputPath가 결과에 포함되어야 함")
	}
	if !strings.Contains(result, "mkdir -p") {
		t.Error("outputDir 지정 시 mkdir 지시가 포함되어야 함")
	}
	if !strings.Contains(result, "결과 JSON 형식") {
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

	count := strings.Count(result, "- ")
	if count != len(paths) {
		t.Errorf("리스트 아이템 수 = %d, want %d", count, len(paths))
	}
}

// --- Build 통합 테스트 ---

func TestClaudeMDBuilder_BuildWithCommunicationRules(t *testing.T) {
	builder := &ClaudeMDBuilder{MaxLines: 200}

	commRules := CommunicationRulesWithPaths(
		"",
		"/workspace/.pylon/runtime/pipeline-001/execution-log.json",
		"/workspace/.pylon/runtime/pipeline-001",
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

	if !strings.Contains(result, "## Communication Rules") {
		t.Error("Communication Rules 섹션 헤더가 없음")
	}
	if !strings.Contains(result, "execution-log.json") {
		t.Error("output 경로가 Build 결과에 포함되어야 함")
	}
	if !strings.Contains(result, "mkdir -p") {
		t.Error("outputDir mkdir 지시가 Build 결과에 포함되어야 함")
	}

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
