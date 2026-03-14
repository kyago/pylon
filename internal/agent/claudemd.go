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
	CommunicationRules string   // Priority 1: inbox/outbox protocol (~30 lines)
	TaskContext        string   // Priority 2: acceptance criteria, constraints (~50 lines)
	CompactionRules    string   // Priority 3: context management rules (~20 lines)
	ProjectMemory      string   // Priority 4: proactive memory summary (~80 lines)
	DomainPaths        []string // Priority 5: domain knowledge file paths (~20 lines)
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
		{"Project Memory", input.ProjectMemory},
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

// DefaultCommunicationRules returns the standard inbox/outbox protocol rules.
// Used for PO/index agents where concrete paths are not needed.
// Spec Reference: Section 8 "CLAUDE.md Injection" communication rules
func DefaultCommunicationRules() string {
	return CommunicationRulesWithPaths("", "", "")
}

// CommunicationRulesWithPaths returns inbox/outbox protocol rules with concrete absolute paths.
// When outboxPath is non-empty, the agent receives an exact file path to write results to.
func CommunicationRulesWithPaths(inboxPath, outboxPath, outboxDir string) string {
	outboxInstruction := `2. 작업 완료 후 결과를 ` + "`" + `.pylon/runtime/outbox/{에이전트명}/{task-id}.tmp.json` + "`" + `에 JSON으로 작성합니다
3. 작성 완료 후 mv 명령을 실행합니다:
   ` + "```" + `bash
   mv .pylon/runtime/outbox/{에이전트명}/{task-id}.tmp.json \
      .pylon/runtime/outbox/{에이전트명}/{task-id}.result.json
   ` + "```"

	if outboxPath != "" {
		tmpPath := strings.TrimSuffix(outboxPath, ".result.json") + ".tmp.json"
		outboxInstruction = fmt.Sprintf(`2. 작업 완료 후 결과를 아래 경로에 JSON으로 작성합니다:
   - 임시 파일: `+"`%s`"+`
3. 작성 완료 후 mv 명령을 실행합니다:
   `+"```"+`bash
   mv %s %s
   `+"```", tmpPath, tmpPath, outboxPath)
	}

	// Ensure outbox directory exists instruction
	mkdirInstruction := ""
	if outboxDir != "" {
		mkdirInstruction = fmt.Sprintf("\n   (디렉토리가 없으면 먼저 생성: `mkdir -p %s`)", outboxDir)
	}

	return `### 파일 기반 메시지 프로토콜
- 태스크를 완료하면 반드시 다음 절차를 따르세요:

1. inbox에서 태스크 파일을 읽어 작업을 수행합니다
` + outboxInstruction + mkdirInstruction + `
4. 절대로 SQLite(pylon.db)에 직접 접근하지 마세요

### 결과 파일 JSON 형식
다음 형식으로 작성하세요:
` + "```json" + `
{
  "task_id": "수행한 태스크 ID",
  "status": "completed",
  "summary": "작업 결과 요약",
  "files_changed": ["변경한 파일 목록"],
  "commits": ["생성한 커밋 해시 (git log --oneline -1 으로 확인)"],
  "learnings": ["작업 중 발견한 교훈/패턴"]
}
` + "```" + `
- status: "completed" | "failed" | "blocked"
- learnings: 프로젝트 메모리에 축적되어 다음 태스크에서 재활용됩니다`
}

// DefaultCompactionRules returns the standard context management rules.
func DefaultCompactionRules() string {
	return `### 컨텍스트 관리
- inbox 메시지의 context 필드를 반드시 참조하세요:
  - summary: 이전 단계 요약
  - decisions: 내려진 아키텍처 결정
  - references: 참고할 문서 경로
- 컨텍스트가 부족하면 query 메시지를 작성하세요`
}
