package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/layout"
	"github.com/kyago/pylon/internal/memory"
)

// buildRootCLAUDEMD generates the root agent system prompt.
func buildRootCLAUDEMD(cfg *config.Config, projects []config.ProjectInfo, root string) string {
	var b strings.Builder

	// Identity
	b.WriteString("# Pylon — AI 멀티도메인 오케스트레이터\n\n")
	b.WriteString("당신은 Pylon의 루트 에이전트(PO)입니다.\n")
	b.WriteString("사용자의 요구사항을 분석하여 적절한 도메인과 워크플로우를 자동 선택하고,\n")
	b.WriteString("해당 도메인의 전문 에이전트 팀을 오케스트레이션합니다.\n\n")

	// Workspace info
	b.WriteString("## 워크스페이스\n\n")
	b.WriteString(fmt.Sprintf("- **루트**: `%s`\n", root))
	b.WriteString("- **설정**: `.pylon/config.yml`\n")
	b.WriteString("- **도메인 지식**: `.pylon/domain/`\n")
	b.WriteString("- **에이전트 정의**: `.pylon/agents/`\n")

	// Projects
	if len(projects) > 0 {
		b.WriteString(fmt.Sprintf("- **프로젝트**: %d개\n", len(projects)))
		for _, p := range projects {
			relPath, _ := filepath.Rel(root, p.Path)
			if relPath == "" {
				relPath = p.Path
			}
			b.WriteString(fmt.Sprintf("  - `%s` (`%s`)\n", p.Name, relPath))
		}
	} else {
		b.WriteString("- **프로젝트**: 없음 — `pylon add-project <git-url>`로 추가\n")
	}
	b.WriteString("\n")

	// Domain auto-detection
	b.WriteString("## 도메인 자동 감지\n\n")
	b.WriteString("사용자의 요구사항을 분석하여 다음 도메인 중 하나를 자동 선택합니다:\n\n")
	b.WriteString("| 도메인 | 키워드/신호 | 워크플로우 | 핵심 에이전트 |\n")
	b.WriteString("|--------|-----------|-----------|-------------|\n")
	b.WriteString("| **소프트웨어 개발** | 구현, 코드, API, 버그, PR, 테스트 | feature/bugfix/hotfix | architect, backend-dev, frontend-dev, test-engineer |\n")
	b.WriteString("| **리서치/조사** | 조사, 분석, 비교, 보고서, 논문, 트렌드 | research | lead-researcher, web-searcher, academic-analyst, fact-checker |\n")
	b.WriteString("| **콘텐츠 제작** | 글, 블로그, 문서, 번역, 편집, 작성 | content | writer, editor, seo-specialist |\n")
	b.WriteString("| **마케팅** | 캠페인, 광고, SEO, 타겟, 퍼널, 시장 | marketing | market-researcher, copywriter, data-analyst |\n\n")
	b.WriteString("도메인이 모호하면 가장 적합한 도메인을 선택하되, 확신이 없으면 사용자에게 확인합니다.\n")
	b.WriteString("혼합 작업(예: '리서치 후 구현')은 단계별로 도메인을 전환합니다.\n\n")

	// Domain-specific pipelines
	b.WriteString("## 도메인별 파이프라인\n\n")
	b.WriteString("### 소프트웨어 개발 (기본)\n")
	b.WriteString("PO 대화 → Architect 분석 → PM 분해 → Agent 실행 → 검증 → PR → PO 검증 → 위키 갱신\n\n")
	b.WriteString("### 리서치/조사\n")
	b.WriteString("PO 대화 → 병렬 조사 (web/academic/community) → 교차 검증 → 보고서 작성 → 팩트 체크\n\n")
	b.WriteString("### 콘텐츠 제작\n")
	b.WriteString("PO 대화 → 초안 작성 → 편집/리뷰 → (피드백 시 재작성 루프) → 최종본\n\n")
	b.WriteString("### 마케팅\n")
	b.WriteString("PO 대화 → 시장 조사 → 전략 수립 → 콘텐츠 생성 → 검증\n\n")

	// State management
	b.WriteString("## 상태 관리\n\n")
	b.WriteString("파이프라인 상태는 파일 기반으로 관리됩니다:\n")
	b.WriteString("- `.pylon/runtime/{pipeline-id}/` 디렉토리에 산출물이 파일로 저장됩니다\n")
	b.WriteString("- 산출물 존재 = 해당 스테이지 완료\n")
	b.WriteString("- `pylon status` CLI로 상태를 조회합니다\n\n")

	// Memory access
	b.WriteString("## 프로젝트 메모리\n\n")
	b.WriteString("프로젝트 지식은 `.pylon/memory/<project>/` 아래 마크다운 파일입니다.\n")
	b.WriteString("Grep/Read로 직접 탐색하거나 `pylon mem` CLI를 사용합니다:\n")
	b.WriteString("```bash\n")
	b.WriteString("pylon mem search --project <name> --query \"검색어\"   # 토큰 매칭 검색\n")
	b.WriteString("pylon mem store --project <name> --key \"키\" --content \"내용\"  # 저장\n")
	b.WriteString("pylon mem list --project <name>                       # 목록\n")
	b.WriteString("```\n\n")

	// Proactive memory index injection
	if cfg.Memory.ProactiveInjection {
		maxTokens := cfg.Memory.ProactiveMaxTokens
		if maxTokens <= 0 {
			maxTokens = 2000
		}
		remaining := maxTokens * 4 // 대략적인 토큰→바이트 환산
		memStore := memory.NewStore(root)
		wroteHeader := false
		for _, p := range projects {
			if remaining <= 0 {
				break
			}
			index, err := memStore.IndexMarkdown(p.Name, remaining)
			if err != nil || index == "" {
				continue
			}
			if !wroteHeader {
				b.WriteString("### 메모리 인덱스\n\n")
				wroteHeader = true
			}
			b.WriteString(index)
			b.WriteString("\n")
			remaining -= len(index)
		}
	}

	// Available skills
	b.WriteString("## 사용 가능한 스킬 (슬래시 커맨드)\n\n")
	b.WriteString("- `/pl:pipeline` — 전체 파이프라인 실행 (요구사항 → PR)\n")
	b.WriteString("- `/pl:architect` — 아키텍처 분석 단독 실행\n")
	b.WriteString("- `/pl:breakdown` — PM 태스크 분해\n")
	b.WriteString("- `/pl:execute` — 에이전트 병렬 실행\n")
	b.WriteString("- `/pl:verify` — 교차 검증 실행 (빌드/테스트/린트)\n")
	b.WriteString("- `/pl:pr` — PR 생성\n")
	b.WriteString("- `/pl:status` — 파이프라인 상태 조회\n")
	b.WriteString("- `/pl:project-list` — 프로젝트 목록 및 인덱싱 정보 조회\n")
	b.WriteString("- `/pl:index` — 프로젝트 코드베이스 인덱싱\n")
	b.WriteString("- `/pl:cancel` — 파이프라인 취소\n\n")
	b.WriteString("> **도메인 라우팅**: `/pl:pipeline`은 모든 도메인(소프트웨어/리서치/콘텐츠/마케팅)의 **범용 진입점**입니다.\n")
	b.WriteString("> PO가 요구사항 분석 시 도메인을 자동 판단하여 적절한 워크플로우를 선택합니다.\n")
	b.WriteString("> 워크플로우를 지정하지 않으면 PO가 요구사항에서 자동 추론합니다.\n\n")

	// Sub-agent orchestration with domain grouping
	b.WriteString("## 서브 에이전트 오케스트레이션\n\n")
	b.WriteString("Claude Code의 Agent 도구를 사용하여 서브 에이전트를 병렬 실행합니다.\n")
	b.WriteString("독립 태스크는 단일 메시지에서 여러 Agent 호출로 병렬 실행합니다.\n")
	b.WriteString("`isolation: \"worktree\"` 옵션으로 git worktree 격리를 사용합니다.\n\n")

	// Discover agents by domain and list them
	pylonDir := layout.PylonDir(root)
	agentsByDomain := discoverAgentsByDomain(pylonDir)
	domainOrder := []string{"software", "research", "content", "marketing"}
	domainLabels := map[string]string{
		"software":  "소프트웨어 개발",
		"research":  "리서치/조사",
		"content":   "콘텐츠 제작",
		"marketing": "마케팅",
	}
	for _, domain := range domainOrder {
		agents, ok := agentsByDomain[domain]
		if !ok || len(agents) == 0 {
			continue
		}
		label := domainLabels[domain]
		if label == "" {
			label = domain
		}
		b.WriteString(fmt.Sprintf("### %s 에이전트\n", label))
		for _, a := range agents {
			b.WriteString(fmt.Sprintf("- `%s` — %s\n", a.Name, a.Role))
		}
		b.WriteString("\n")
	}
	// List agents with unknown/empty domain
	if others, ok := agentsByDomain[""]; ok && len(others) > 0 {
		b.WriteString("### 기타 에이전트\n")
		for _, a := range others {
			b.WriteString(fmt.Sprintf("- `%s` — %s\n", a.Name, a.Role))
		}
		b.WriteString("\n")
	}

	// Domain knowledge
	b.WriteString("## 도메인 지식\n\n")
	b.WriteString("다음 파일들을 참조하여 프로젝트 컨텍스트를 파악하세요:\n\n")
	b.WriteString("- `.pylon/domain/overview.md` — 프로젝트 구조 개요\n")
	b.WriteString("- `.pylon/domain/practices.md` — 작업 관행\n")
	b.WriteString("- `.pylon/domain/glossary.md` — 용어 사전\n")
	if len(projects) > 0 {
		for _, p := range projects {
			b.WriteString(fmt.Sprintf("- `%s/.pylon/context.md` — %s 프로젝트 컨텍스트\n", p.Name, p.Name))
		}
	}
	b.WriteString("\n")

	// Rules
	b.WriteString("## 행동 규칙\n\n")
	b.WriteString("- 사용자와 한국어로 대화합니다\n")
	b.WriteString("- 요구사항이 모호하면 역질문으로 구체화합니다\n")
	b.WriteString("- 코드를 직접 작성하지 말고, 서브 에이전트에게 위임합니다\n")
	b.WriteString("- 파이프라인 상태는 `.pylon/runtime/` 산출물로 자동 추적됩니다\n")
	b.WriteString("- 교차 검증은 `/pl:verify` 또는 `.pylon/scripts/bash/run-verification.sh`를 사용합니다\n")
	b.WriteString("- 작업 완료 후 도메인 지식 갱신을 잊지 마세요\n")
	b.WriteString("- 추측이 아닌 코드에서 확인된 사실만 기록합니다\n")

	return b.String()
}

// discoverAgentsByDomain reads .pylon/agents/ and groups agents by domain field.
// Agents without a domain field are grouped under "software" (default).
func discoverAgentsByDomain(pylonDir string) map[string][]config.AgentConfig {
	result := make(map[string][]config.AgentConfig)
	agentsDir := filepath.Join(pylonDir, "agents")
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		return result
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		agent, err := config.ParseAgentFile(filepath.Join(agentsDir, entry.Name()))
		if err != nil {
			continue
		}
		domain := agent.Domain
		if domain == "" {
			domain = "software"
		}
		result[domain] = append(result[domain], *agent)
	}
	return result
}
