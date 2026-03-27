package agent

import (
	"fmt"
	"strings"
)

const defaultMaxLines = 200

// ClaudeMDBuilder generates CLAUDE.md content for agent injection.
// Spec Reference: Section 8 "Agent CLAUDE.md Injection Rules" (200-line limit)
type ClaudeMDBuilder struct {
	MaxLines int
}

// BuildInput defines the 5-level priority content for CLAUDE.md.
// Priority order: 1 (highest) to 5 (lowest).
type BuildInput struct {
	CommunicationRules string   // Priority 1: agent execution rules (~30 lines)
	TaskContext        string   // Priority 2: acceptance criteria, constraints (~50 lines)
	CompactionRules    string   // Priority 3: context management rules (~20 lines)
	DomainPaths        []string // Priority 4: domain knowledge file paths (~20 lines)
}

// Build constructs the CLAUDE.md content within the line limit.
// If total exceeds MaxLines, lower-priority sections are truncated first.
func (b *ClaudeMDBuilder) Build(input BuildInput) (string, error) {
	maxLines := b.MaxLines
	if maxLines <= 0 {
		maxLines = defaultMaxLines
	}

	// Build sections in priority order
	sections := []struct {
		name    string
		content string
	}{
		{"Communication Rules", input.CommunicationRules},
		{"Task Context", input.TaskContext},
		{"Context Management", input.CompactionRules},
		{"Domain Knowledge", buildDomainSection(input.DomainPaths)},
	}

	var result []string
	linesUsed := 0

	for _, sec := range sections {
		if sec.content == "" {
			continue
		}

		sectionLines := strings.Split(sec.content, "\n")
		sectionHeader := fmt.Sprintf("## %s", sec.name)
		needed := len(sectionLines) + 2 // header + blank line

		if linesUsed+needed > maxLines {
			// Truncate this section to fit
			remaining := maxLines - linesUsed - 2 // reserve for header + blank
			if remaining <= 0 {
				break // no room for any more sections
			}
			result = append(result, sectionHeader)
			result = append(result, sectionLines[:remaining]...)
			result = append(result, "")
			break // stop adding sections
		}

		result = append(result, sectionHeader)
		result = append(result, sectionLines...)
		result = append(result, "")
		linesUsed += needed
	}

	return strings.Join(result, "\n"), nil
}

// buildDomainSection creates the domain knowledge reference section.
// Only file paths are included, not actual content — agents read files as needed.
func buildDomainSection(paths []string) string {
	if len(paths) == 0 {
		return ""
	}

	var lines []string
	lines = append(lines, "참조할 도메인 지식 문서:")
	for _, p := range paths {
		lines = append(lines, fmt.Sprintf("- %s", p))
	}
	lines = append(lines, "")
	lines = append(lines, "필요한 경우 위 파일을 직접 읽어 참고하세요.")
	return strings.Join(lines, "\n")
}

// DefaultCommunicationRules returns the standard v2 agent output rules.
func DefaultCommunicationRules() string {
	return CommunicationRulesWithPaths("", "", "")
}

// CommunicationRulesWithPaths returns agent output rules with optional pipeline directory path.
// In v2, agents write results as artifacts in the pipeline runtime directory.
func CommunicationRulesWithPaths(_, outputPath, outputDir string) string {
	outputInstruction := `2. 작업 완료 후 결과를 파이프라인 런타임 디렉토리에 산출물로 저장합니다`
	if outputPath != "" {
		outputInstruction = fmt.Sprintf("2. 작업 완료 후 결과를 `%s`에 저장합니다", outputPath)
	}

	mkdirInstruction := ""
	if outputDir != "" {
		mkdirInstruction = fmt.Sprintf("\n   (디렉토리가 없으면 먼저 생성: `mkdir -p %s`)", outputDir)
	}

	return `### 에이전트 실행 규칙
- 태스크를 완료하면 반드시 다음 절차를 따르세요:

1. 할당된 태스크를 수행합니다
` + outputInstruction + mkdirInstruction + `
3. 절대로 SQLite(pylon.db)에 직접 접근하지 마세요

### 결과 JSON 형식
` + "```json" + `
{
  "task_id": "수행한 태스크 ID",
  "status": "completed",
  "summary": "작업 결과 요약",
  "files_changed": ["변경한 파일 목록"],
  "commits": ["생성한 커밋 해시"],
  "learnings": ["작업 중 발견한 교훈/패턴"]
}
` + "```" + `
- status: "completed" | "failed" | "blocked"
- learnings: 프로젝트 메모리에 축적되어 다음 태스크에서 재활용됩니다`
}

// DefaultCompactionRules returns the standard context management rules.
func DefaultCompactionRules() string {
	return `### 컨텍스트 관리
- 태스크의 context 필드를 반드시 참조하세요:
  - summary: 이전 단계 요약
  - decisions: 내려진 아키텍처 결정
  - references: 참고할 문서 경로
- 컨텍스트가 부족하면 필요한 파일을 직접 읽어 참고하세요`
}
