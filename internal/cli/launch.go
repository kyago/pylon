package cli

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/charmbracelet/huh"
	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/store"
)

//go:embed hooks.json
var defaultHooksJSON []byte

//go:embed commands/*.md
var embeddedCommands embed.FS

// runLaunch is the main entry point when `pylon` is invoked without subcommands.
// It generates .claude/ artifacts from .pylon/ (source of truth) and launches
// Claude Code TUI directly via syscall.Exec.
func runLaunch() error {
	// Step 1: Find workspace
	startDir := flagWorkspace
	if startDir == "" {
		startDir = "."
	}
	root, err := config.FindWorkspaceRoot(startDir)
	if err != nil {
		return fmt.Errorf("pylon 워크스페이스가 아닙니다 — 'pylon init'을 먼저 실행하세요")
	}

	// Step 2: Load config
	cfg, err := config.LoadConfig(filepath.Join(root, ".pylon", "config.yml"))
	if err != nil {
		return fmt.Errorf("설정 로드 실패: %w", err)
	}

	// Step 3: Discover projects
	projects, err := config.DiscoverProjects(root)
	if err != nil {
		// Non-fatal: workspace may not have projects yet
		projects = nil
	}

	// Step 4: Generate .claude/ directory structure
	if err := generateClaudeDir(root, cfg, projects); err != nil {
		return fmt.Errorf(".claude/ 생성 실패: %w", err)
	}

	// Ensure .claude/ and CLAUDE.md are in .gitignore
	ensureGitignore(root)

	// Step 5: Select permission mode
	permMode, err := selectPermissionMode(cfg.Runtime.PermissionMode)
	if err != nil {
		return err
	}

	// Step 6: Launch Claude Code (replace process)
	claudePath, err := exec.LookPath("claude")
	if err != nil {
		return fmt.Errorf("claude 실행 파일을 찾을 수 없습니다: %w", err)
	}

	args := append([]string{"claude"}, buildClaudeArgs(cfg, permMode)...)

	// Build env with config overrides (deduplicated — config wins over existing)
	envMap := make(map[string]string)
	for _, e := range os.Environ() {
		if k, v, ok := strings.Cut(e, "="); ok {
			envMap[k] = v
		}
	}
	for k, v := range cfg.Runtime.Env {
		envMap[k] = v
	}
	env := make([]string, 0, len(envMap))
	for k, v := range envMap {
		env = append(env, k+"="+v)
	}

	fmt.Println("Claude Code를 시작합니다...")
	return syscall.Exec(claudePath, args, env)
}


// openWorkspaceStore is a shared helper that finds the workspace root, loads config,
// and opens the SQLite store. Caller must close the returned Store.
func openWorkspaceStore() (string, *config.Config, *store.Store, error) {
	startDir := flagWorkspace
	if startDir == "" {
		startDir = "."
	}
	root, err := config.FindWorkspaceRoot(startDir)
	if err != nil {
		return "", nil, nil, fmt.Errorf("not in a pylon workspace")
	}

	cfg, err := config.LoadConfig(filepath.Join(root, ".pylon", "config.yml"))
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to load config: %w", err)
	}

	dbPath := filepath.Join(root, ".pylon", "pylon.db")
	s, err := store.NewStore(dbPath)
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to open store: %w", err)
	}

	if err := s.Migrate(); err != nil {
		s.Close()
		return "", nil, nil, fmt.Errorf("failed to migrate: %w", err)
	}

	return root, cfg, s, nil
}

// selectPermissionMode presents an interactive selector for Claude Code permission mode.
func selectPermissionMode(defaultMode string) (string, error) {
	if defaultMode == "" {
		defaultMode = "default"
	}

	modes := []huh.Option[string]{
		huh.NewOption("default — 매번 권한 확인", "default"),
		huh.NewOption("acceptEdits — 파일 편집 자동 허용", "acceptEdits"),
		huh.NewOption("bypassPermissions — 모든 권한 자동 허용", "bypassPermissions"),
	}

	// Pre-select the default from config
	for i, m := range modes {
		if m.Value == defaultMode {
			modes[i] = modes[i].Selected(true)
			break
		}
	}

	var selected string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Permission Mode 선택").
				Description("Claude Code 실행 권한을 설정합니다").
				Options(modes...).
				Value(&selected),
		),
	)

	if err := form.Run(); err != nil {
		return "", fmt.Errorf("선택 취소됨: %w", err)
	}

	return selected, nil
}

// buildClaudeArgs constructs the claude CLI arguments as a string slice.
func buildClaudeArgs(cfg *config.Config, permMode string) []string {
	var args []string

	if cfg.Runtime.MaxTurns > 0 {
		args = append(args, "--max-turns", fmt.Sprintf("%d", cfg.Runtime.MaxTurns))
	}
	args = append(args, "--permission-mode", permMode)

	return args
}

// generateClaudeDir creates/updates the .claude/ directory from .pylon/ source of truth.
func generateClaudeDir(root string, cfg *config.Config, projects []config.ProjectInfo) error {
	claudeDir := filepath.Join(root, ".claude")
	commandsDir := filepath.Join(claudeDir, "commands")

	// Ensure directories exist
	if err := os.MkdirAll(commandsDir, 0755); err != nil {
		return err
	}

	// Generate CLAUDE.md at workspace root
	claudeMD := buildRootCLAUDEMD(cfg, projects, root)
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte(claudeMD), 0644); err != nil {
		return fmt.Errorf("CLAUDE.md 생성 실패: %w", err)
	}

	// Clean up legacy command files (pre-namespace) to prevent stale slash commands
	legacyCommands := []string{"index", "status", "verify", "add-project", "cancel", "review"}
	for _, name := range legacyCommands {
		legacy := filepath.Join(commandsDir, name+".md")
		if err := os.Remove(legacy); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("레거시 커맨드 파일 제거 실패 (%s): %w", legacy, err)
		}
	}

	// Generate slash commands from Go code
	commands := buildSlashCommands(root)
	for name, content := range commands {
		cmdPath := filepath.Join(commandsDir, filepath.FromSlash(name)+".md")
		if err := os.MkdirAll(filepath.Dir(cmdPath), 0755); err != nil {
			return fmt.Errorf("커맨드 디렉토리 생성 실패: %w", err)
		}
		if err := os.WriteFile(cmdPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("커맨드 %s 생성 실패: %w", name, err)
		}
	}

	// Pipeline slash commands → .claude/commands/pl/
	// Priority: .pylon/commands/ (user customization) > embedded defaults
	plDir := filepath.Join(commandsDir, "pl")
	if err := os.MkdirAll(plDir, 0755); err != nil {
		return fmt.Errorf("pl/ 디렉토리 생성 실패: %w", err)
	}

	pylonCmdsDir := filepath.Join(root, ".pylon", "commands")
	if entries, err := os.ReadDir(pylonCmdsDir); err == nil && len(entries) > 0 {
		// Use workspace .pylon/commands/ (user may have customized)
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			content, err := os.ReadFile(filepath.Join(pylonCmdsDir, entry.Name()))
			if err != nil {
				continue
			}
			destName := strings.TrimPrefix(entry.Name(), "pl-")
			if err := os.WriteFile(filepath.Join(plDir, destName), content, 0644); err != nil {
				return fmt.Errorf("커맨드 복사 실패 (%s): %w", entry.Name(), err)
			}
		}
	} else {
		// Fallback: use embedded default commands (existing workspaces without .pylon/commands/)
		embedded, _ := embeddedCommands.ReadDir("commands")
		for _, entry := range embedded {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			content, err := embeddedCommands.ReadFile("commands/" + entry.Name())
			if err != nil {
				continue
			}
			destName := strings.TrimPrefix(entry.Name(), "pl-")
			if err := os.WriteFile(filepath.Join(plDir, destName), content, 0644); err != nil {
				return fmt.Errorf("내장 커맨드 생성 실패 (%s): %w", entry.Name(), err)
			}
		}
		// Also bootstrap .pylon/commands/ for future customization
		os.MkdirAll(pylonCmdsDir, 0755)
		for _, entry := range embedded {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			content, _ := embeddedCommands.ReadFile("commands/" + entry.Name())
			_ = os.WriteFile(filepath.Join(pylonCmdsDir, entry.Name()), content, 0644)
		}
	}

	// Pipeline scripts → .pylon/scripts/bash/
	// Bootstrap embedded scripts if workspace doesn't have them yet (existing workspaces)
	pylonScriptsDir := filepath.Join(root, ".pylon", "scripts", "bash")
	if err := os.MkdirAll(pylonScriptsDir, 0755); err != nil {
		return fmt.Errorf("scripts/bash/ 디렉토리 생성 실패: %w", err)
	}
	scriptEntries, err := embeddedScripts.ReadDir("scripts/bash")
	if err == nil {
		for _, entry := range scriptEntries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sh") {
				continue
			}
			destPath := filepath.Join(pylonScriptsDir, entry.Name())
			// Only write if file doesn't exist (preserve user customizations)
			if _, err := os.Stat(destPath); os.IsNotExist(err) {
				content, err := embeddedScripts.ReadFile("scripts/bash/" + entry.Name())
				if err != nil {
					fmt.Fprintf(os.Stderr, "경고: 내장 스크립트 읽기 실패 (%s): %v\n", entry.Name(), err)
					continue
				}
				if err := os.WriteFile(destPath, content, 0755); err != nil {
					fmt.Fprintf(os.Stderr, "경고: 스크립트 배포 실패 (%s): %v\n", entry.Name(), err)
				}
			}
		}
	}

	// Generate .claude/agents/ with skill injection
	if err := generateClaudeAgentsWithSkills(root, cfg); err != nil {
		return fmt.Errorf(".claude/agents/ 생성 실패: %w", err)
	}

	// Generate hooks in .claude/settings.json for Claude Code session lifecycle
	if err := generateSettingsHooks(claudeDir); err != nil {
		return fmt.Errorf("settings.json hooks 생성 실패: %w", err)
	}

	// Generate .mcp.json for pylon-ontology MCP server if enabled
	if cfg.Ontology.Enabled {
		if err := generateOntologyMCPConfig(root, cfg); err != nil {
			return fmt.Errorf(".mcp.json 온톨로지 설정 실패: %w", err)
		}
	}

	return nil
}

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
	b.WriteString(fmt.Sprintf("- **설정**: `.pylon/config.yml`\n"))
	b.WriteString(fmt.Sprintf("- **도메인 지식**: `.pylon/domain/`\n"))
	b.WriteString(fmt.Sprintf("- **에이전트 정의**: `.pylon/agents/`\n"))

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
	b.WriteString("프로젝트 지식은 `pylon mem` CLI를 사용합니다:\n")
	b.WriteString("```bash\n")
	b.WriteString("pylon mem search --project <name> --query \"검색어\"   # BM25 검색\n")
	b.WriteString("pylon mem store --project <name> --key \"키\" --content \"내용\"  # 저장\n")
	b.WriteString("pylon mem list --project <name>                       # 목록\n")
	b.WriteString("```\n\n")

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

	// Sub-agent orchestration with domain grouping
	b.WriteString("## 서브 에이전트 오케스트레이션\n\n")
	b.WriteString("Claude Code의 Agent 도구를 사용하여 서브 에이전트를 병렬 실행합니다.\n")
	b.WriteString("독립 태스크는 단일 메시지에서 여러 Agent 호출로 병렬 실행합니다.\n")
	b.WriteString("`isolation: \"worktree\"` 옵션으로 git worktree 격리를 사용합니다.\n\n")

	// Discover agents by domain and list them
	pylonDir := filepath.Join(root, ".pylon")
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
	b.WriteString("- `.pylon/domain/architecture.md` — 시스템 아키텍처\n")
	b.WriteString("- `.pylon/domain/conventions.md` — 코딩 컨벤션\n")
	b.WriteString("- `.pylon/domain/glossary.md` — 비즈니스 용어 사전\n")
	if len(projects) > 0 {
		for _, p := range projects {
			b.WriteString(fmt.Sprintf("- `%s/.pylon/context.md` — %s 프로젝트 컨텍스트\n", p.Name, p.Name))
		}
	}
	b.WriteString("\n")

	// Ontology integration (MCP server)
	if cfg.Ontology.Enabled {
		ontologyPackageName := cfg.Ontology.PackageName
		if ontologyPackageName == "" {
			ontologyPackageName = "pylon-ontology"
		}

		b.WriteString(fmt.Sprintf("## 온톨로지 자동화 (%s)\n\n", ontologyPackageName))
		b.WriteString(fmt.Sprintf("이 워크스페이스는 %s MCP 서버가 연동되어 있습니다.\n", ontologyPackageName))
		b.WriteString("코드에서 도메인 용어, 아키텍처 결정, 코딩 컨벤션을 자동으로 추출·축적합니다.\n\n")
		b.WriteString("### 사용 가능한 온톨로지 도구\n\n")
		b.WriteString("- `extract_ontology` — 파일에서 구조적 심볼을 AST 파싱으로 추출\n")
		b.WriteString("- `add_term` / `add_decision` / `add_convention` — 온톨로지 항목 등록\n")
		b.WriteString("- `query_ontology` — FTS5+BM25 키워드 검색\n")
		b.WriteString("- `verify_ontology` — stale/conflict 감지 (liveness check)\n")
		b.WriteString("- `export_ontology` — YAML/JSON 내보내기\n")
		b.WriteString("- `get_context` — 현재 온톨로지 요약을 마크다운으로 조회\n\n")
		b.WriteString("### 워크플로우 통합\n\n")
		if cfg.Ontology.AutoExtract {
			b.WriteString("- 파일 편집/생성 이후 필요 시 `extract_ontology` 도구를 호출해 온톨로지를 갱신하세요 (자동 훅 연동은 추후 지원 예정)\n")
		}
		if cfg.Ontology.AutoVerify {
			b.WriteString("- 파이프라인 완료 후 `verify_ontology`를 실행해 부패한 항목을 점검하는 것이 권장됩니다 (자동 실행은 추후 지원 예정)\n")
		}
		b.WriteString("- `.pylon/domain/` 디렉터리는 도메인 위키/핵심 지식 저장소이며, 온톨로지 DB(`.pylon/ontology.db`)는 이를 보완하는 구조화된 인덱스입니다\n\n")
	}

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

// buildSlashCommands generates .claude/commands/pl/ markdown files.
func buildSlashCommands(root string) map[string]string {
	commands := map[string]string{
		"pl/project-list": `# /pl:project-list — 프로젝트 목록 및 인덱싱 정보 조회

워크스페이스에 등록된 프로젝트들의 목록과 인덱싱 정보를 조회하여 사용자에게 제공합니다.

## 절차

1. 프로젝트 목록을 동기화하고 조회합니다 (DB에 최신 상태를 반영하기 위해 sync를 먼저 실행):
   ` + "```" + `bash
   pylon sync-projects
   ` + "```" + `

2. 각 프로젝트의 인덱싱 상태를 판단합니다. 다음 두 지표를 확인하여 판단합니다:

   a. 컨텍스트 파일 존재 여부 (인덱싱의 주요 지표):
      - ` + "`" + `<프로젝트명>/.pylon/context.md` + "`" + ` 파일이 존재하면 인덱싱 완료로 판단

   b. 프로젝트 메모리 조회 (카테고리 필터 없이 전체 조회):
      ` + "```" + `bash
      pylon mem list --project <프로젝트명>
      ` + "```" + `
      결과가 없으면 워크스페이스명으로 재시도합니다 (멀티 프로젝트 워크스페이스에서는 워크스페이스명으로 저장됨):
      ` + "```" + `bash
      pylon mem list --project <워크스페이스명>
      ` + "```" + `

3. 각 프로젝트의 추가 정보를 확인합니다:
   - ` + "`" + `<프로젝트명>/.pylon/context.md` + "`" + ` — 프로젝트 컨텍스트 (존재 여부 및 요약)
   - ` + "`" + `<프로젝트명>/.pylon/agents/` + "`" + ` — 프로젝트 전용 에이전트 정의

4. 수집한 정보를 다음 형식으로 정리하여 사용자에게 보여줍니다:

   **각 프로젝트별 표시 항목:**
   - 프로젝트명
   - 경로
   - 기술 스택 (tech stack)
   - 인덱싱 상태 (context.md 존재 여부 + 메모리 엔트리 수)
   - 컨텍스트 파일 존재 여부
   - 등록된 에이전트 수

## 인덱싱 상태 판단 기준

- **인덱싱 완료**: ` + "`" + `<프로젝트명>/.pylon/context.md` + "`" + ` 파일이 존재하거나, 해당 프로젝트의 메모리 엔트리가 1개 이상 있는 경우
- **미인덱싱**: 위 조건을 모두 만족하지 않는 경우

## 주의사항

- 인덱싱이 되지 않은 프로젝트는 "미인덱싱" 상태로 표시하고 ` + "`" + `/pl:index` + "`" + ` 실행을 안내
- 프로젝트가 없는 경우 ` + "`" + `pylon add-project <git-url>` + "`" + `로 추가하도록 안내
- 출력은 테이블 또는 구조화된 목록 형태로 보기 좋게 정리
- 메모리 조회 시 ` + "`" + `--category` + "`" + ` 필터를 사용하지 않습니다 (실제 데이터는 change, learning 등 다양한 카테고리로 저장됨)
`,

		"pl/index": `# /pl:index — 프로젝트 코드베이스 인덱싱

프로젝트 코드베이스를 분석하여 도메인 위키와 프로젝트 컨텍스트를 갱신합니다.

## 절차

1. 대상 프로젝트를 사용자에게 확인합니다
2. 각 프로젝트 디렉토리의 구조, 주요 파일, 의존성을 분석합니다
3. ` + "`" + `.pylon/domain/` + "`" + ` 하위에 도메인 지식 문서를 생성/갱신합니다:
   - architecture.md — 아키텍처 개요
   - conventions.md — 코딩 컨벤션
   - glossary.md — 비즈니스 용어 사전
4. 각 프로젝트의 ` + "`" + `{프로젝트}/.pylon/context.md` + "`" + `를 실제 코드에 맞게 갱신합니다
5. 발견한 기술 스택 정보를 메모리에 저장합니다:
   ` + "```" + `bash
   pylon mem store --project <name> --key "tech-stack" --content "..." --category codebase
   ` + "```" + `

## 주의사항

- 기존 내용이 있으면 보존하되, 변경된 부분만 업데이트
- 추측이 아닌 코드에서 확인된 사실만 기록
- 위키 문서는 마크다운 형식으로 작성
`,
	}
	return commands
}


// addToGitignore appends pylon-managed entries to .gitignore if not already present.
func addClaudeDirToGitignore(root string) error {
	gitignorePath := filepath.Join(root, ".gitignore")

	existing, _ := os.ReadFile(gitignorePath)
	content := string(existing)

	// Collect missing entries
	var missing []string
	for _, entry := range []string{".claude/", "CLAUDE.md", ".pylon/logs/"} {
		if !strings.Contains(content, entry) {
			missing = append(missing, entry)
		}
	}
	if len(missing) == 0 {
		return nil
	}

	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	var b strings.Builder
	b.WriteString("\n# Pylon-generated (dynamically generated)\n")
	for _, entry := range missing {
		b.WriteString(entry + "\n")
	}
	_, err = f.WriteString(b.String())
	return err
}

// generateClaudeAgentsWithSkills generates .claude/agents/ files with skill injection.
// For agents with skills: generates a file with skill content appended to the body.
// For agents without skills: creates a symlink to .pylon/agents/ (default behavior).
// Respects SkillsConfig flags: Enabled, PreloadToAgents, ProgressiveDisclosure.
func generateClaudeAgentsWithSkills(root string, cfg *config.Config) error {
	pylonDir := filepath.Join(root, ".pylon")
	pylonAgentsDir := filepath.Join(pylonDir, "agents")
	claudeAgentsDir := filepath.Join(root, ".claude", "agents")
	skillsDir := filepath.Join(pylonDir, "skills")

	if err := os.MkdirAll(claudeAgentsDir, 0755); err != nil {
		return err
	}

	entries, err := os.ReadDir(pylonAgentsDir)
	if err != nil {
		return nil // no agents directory, nothing to do
	}

	// Preload skill map for efficient lookup
	skillMap := make(map[string]*config.SkillConfig)
	if cfg.Skills.Enabled && cfg.Skills.PreloadToAgents {
		skills, err := config.DiscoverSkills(skillsDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "경고: 스킬 로드 실패: %v\n", err)
		}
		for _, s := range skills {
			skillMap[s.Name] = s
		}
	}

	// Track which agent files are expected so we can clean up stale entries
	expectedAgents := make(map[string]bool)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		expectedAgents[entry.Name()] = true
		agentPath := filepath.Join(pylonAgentsDir, entry.Name())
		linkPath := filepath.Join(claudeAgentsDir, entry.Name())

		// Parse agent to check for skills
		agent, err := config.ParseAgentFile(agentPath)
		if err != nil {
			// Unparseable agent — fall back to symlink
			ensureSymlink(linkPath, filepath.Join("..", "..", ".pylon", "agents", entry.Name()))
			continue
		}

		// If skills disabled or agent has no skills, use symlink
		if !cfg.Skills.Enabled || !cfg.Skills.PreloadToAgents || len(agent.Skills) == 0 {
			ensureSymlink(linkPath, filepath.Join("..", "..", ".pylon", "agents", entry.Name()))
			continue
		}

		// Agent has skills — generate file with skill content injected
		content, err := os.ReadFile(agentPath)
		if err != nil {
			continue
		}

		injected := buildSkillInjection(agent.Skills, skillMap, cfg.Skills.ProgressiveDisclosure)
		if injected == "" {
			// No matching skills found, use symlink
			ensureSymlink(linkPath, filepath.Join("..", "..", ".pylon", "agents", entry.Name()))
			continue
		}

		// Append skill section to agent content
		combined := string(content) + "\n\n" + injected

		// Remove existing symlink only; preserve user-created regular files
		if info, err := os.Lstat(linkPath); err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				os.Remove(linkPath)
			} else {
				// Regular file exists — don't overwrite user files
				continue
			}
		}
		if err := os.WriteFile(linkPath, []byte(combined), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "경고: %s 에이전트 파일 생성 실패: %v\n", entry.Name(), err)
		}
	}

	// Clean up stale entries in .claude/agents/ that no longer exist in .pylon/agents/
	claudeEntries, _ := os.ReadDir(claudeAgentsDir)
	for _, ce := range claudeEntries {
		if ce.IsDir() || !strings.HasSuffix(ce.Name(), ".md") {
			continue
		}
		if expectedAgents[ce.Name()] {
			continue
		}
		stale := filepath.Join(claudeAgentsDir, ce.Name())
		info, err := os.Lstat(stale)
		if err != nil {
			continue
		}
		// Only remove symlinks and pylon-generated files (not user files)
		if info.Mode()&os.ModeSymlink != 0 {
			os.Remove(stale)
		}
	}

	return nil
}

// buildSkillInjection generates the skill content to inject into an agent file.
// If progressiveDisclosure is true, only metadata (name + description) is injected.
// If false, the full skill body is injected.
func buildSkillInjection(skillNames []string, skillMap map[string]*config.SkillConfig, progressiveDisclosure bool) string {
	var b strings.Builder
	injected := false

	for _, name := range skillNames {
		skill, ok := skillMap[name]
		if !ok {
			continue
		}

		if !injected {
			b.WriteString("## 주입된 스킬\n\n")
			injected = true
		}

		if progressiveDisclosure {
			// Metadata only: name + description
			b.WriteString(fmt.Sprintf("### %s\n", skill.Name))
			if skill.Description != "" {
				b.WriteString(fmt.Sprintf("%s\n", skill.Description))
			}
			b.WriteString(fmt.Sprintf("\n> 전체 내용은 `.pylon/skills/%s.md`를 참조하세요.\n\n", skill.Name))
		} else {
			// Full body injection
			b.WriteString(fmt.Sprintf("### %s\n", skill.Name))
			if skill.Description != "" {
				b.WriteString(fmt.Sprintf("_%s_\n\n", skill.Description))
			}
			if skill.Body != "" {
				b.WriteString(skill.Body)
				b.WriteString("\n\n")
			}
		}
	}

	return b.String()
}

// ensureSymlink creates a symlink at linkPath pointing to target.
// If a symlink already exists, it is removed and recreated.
// Regular files are left untouched.
func ensureSymlink(linkPath, target string) {
	if info, err := os.Lstat(linkPath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			os.Remove(linkPath)
		} else {
			return // regular file, don't touch
		}
	}
	if err := os.Symlink(target, linkPath); err != nil && !os.IsExist(err) {
		fmt.Fprintf(os.Stderr, "경고: 심링크 생성 실패 %s: %v\n", filepath.Base(linkPath), err)
	}
}

// ensureGitignore is called on first launch to add .claude/ to .gitignore.
func ensureGitignore(root string) {
	_ = addClaudeDirToGitignore(root)
}

// formatProjectsJSON returns project info as JSON for agent consumption.
func formatProjectsJSON(projects []config.ProjectInfo) string {
	type projectOut struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	out := make([]projectOut, len(projects))
	for i, p := range projects {
		out[i] = projectOut{Name: p.Name, Path: p.Path}
	}
	data, _ := json.Marshal(out)
	return string(data)
}

// settingsHookCommand represents a single command within a hook group.
type settingsHookCommand struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

// settingsHookEntry represents a hook group in .claude/settings.json.
// Each group has a matcher string and an array of hook commands.
type settingsHookEntry struct {
	Matcher string               `json:"matcher"`
	Hooks   []settingsHookCommand `json:"hooks"`
}

// generateSettingsHooks writes hook definitions into .claude/settings.json.
// It merges with any existing settings to avoid overwriting user-defined config.
// This connects the session lifecycle to pylon's memory system, solving the
// syscall.Exec memory propagation gap.
func generateSettingsHooks(claudeDir string) error {
	settingsPath := filepath.Join(claudeDir, "settings.json")

	// Load existing settings.json if present
	existing := make(map[string]any)
	if data, err := os.ReadFile(settingsPath); err == nil {
		if err := json.Unmarshal(data, &existing); err != nil {
			// If existing file is invalid JSON, start fresh but log warning
			existing = make(map[string]any)
		}
	}

	// Load pylon hook entries from embedded hooks.json
	var pylonHooks map[string][]settingsHookEntry
	if err := json.Unmarshal(defaultHooksJSON, &pylonHooks); err != nil {
		return fmt.Errorf("내장 hooks.json 파싱 실패: %w", err)
	}

	// Merge hooks: preserve non-pylon hooks, replace pylon hooks
	mergedHooks := mergeHooks(existing, pylonHooks)
	existing["hooks"] = mergedHooks

	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return fmt.Errorf("settings.json 직렬화 실패: %w", err)
	}

	if err := os.WriteFile(settingsPath, data, 0644); err != nil {
		return fmt.Errorf("settings.json 쓰기 실패: %w", err)
	}

	// Clean up legacy hooks.json if it exists
	legacyHooksPath := filepath.Join(claudeDir, "hooks.json")
	if err := os.Remove(legacyHooksPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("레거시 hooks.json 제거 실패: %w", err)
	}

	return nil
}

// isPylonHookCommand checks if a command string is a pylon-managed hook.
func isPylonHookCommand(command string) bool {
	return strings.Contains(command, "pylon sync-memory")
}

// isPylonHookGroup checks if a hook group contains any pylon-managed hook commands.
func isPylonHookGroup(entryMap map[string]any) bool {
	hooksArr, ok := entryMap["hooks"].([]any)
	if !ok {
		return false
	}
	for _, h := range hooksArr {
		if hookMap, ok := h.(map[string]any); ok {
			cmd, _ := hookMap["command"].(string)
			if isPylonHookCommand(cmd) {
				return true
			}
		}
	}
	return false
}

// generateOntologyMCPConfig creates or updates .mcp.json with pylon-ontology server config.
// Existing entries are preserved; only the pylon-ontology entry is added/updated.
func generateOntologyMCPConfig(root string, cfg *config.Config) error {
	mcpPath := filepath.Join(root, ".mcp.json")

	existing := make(map[string]any)
	if data, err := os.ReadFile(mcpPath); err == nil {
		if err := json.Unmarshal(data, &existing); err != nil {
			return fmt.Errorf(".mcp.json 파싱 실패: %w (기존 파일을 수정하거나 백업한 뒤 다시 시도하세요)", err)
		}
	}

	// Ensure mcpServers map exists
	servers, ok := existing["mcpServers"].(map[string]any)
	if !ok {
		if _, exists := existing["mcpServers"]; exists {
			return fmt.Errorf(".mcp.json의 mcpServers 필드가 예상하지 못한 형식입니다")
		}
		servers = make(map[string]any)
	}

	// Add pylon-ontology entry
	servers[cfg.Ontology.PackageName] = map[string]any{
		"command": "npx",
		"args":    []string{"--yes", cfg.Ontology.PackageName},
		"env": map[string]string{
			"PYLON_ROOT": root,
		},
	}
	existing["mcpServers"] = servers

	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return fmt.Errorf(".mcp.json 직렬화 실패: %w", err)
	}
	return os.WriteFile(mcpPath, data, 0644)
}

// mergeHooks merges pylon hook entries into existing settings, preserving
// user-defined hooks while replacing pylon-managed ones.
func mergeHooks(existing map[string]any, pylonHooks map[string][]settingsHookEntry) map[string]any {
	result := make(map[string]any)

	// Get existing hooks map if present
	var existingHooks map[string]any
	if h, ok := existing["hooks"]; ok {
		if hm, ok := h.(map[string]any); ok {
			existingHooks = hm
		}
	}

	// For each hook event type in existing, preserve non-pylon entries
	if existingHooks != nil {
		for eventType, entries := range existingHooks {
			var preserved []any
			if arr, ok := entries.([]any); ok {
				for _, entry := range arr {
					if entryMap, ok := entry.(map[string]any); ok {
						if !isPylonHookGroup(entryMap) {
							preserved = append(preserved, entry)
						}
					}
				}
			}
			if len(preserved) > 0 {
				result[eventType] = preserved
			}
		}
	}

	// Add pylon hooks
	for eventType, entries := range pylonHooks {
		var existing []any
		if arr, ok := result[eventType]; ok {
			if a, ok := arr.([]any); ok {
				existing = a
			}
		}
		for _, entry := range entries {
			existing = append(existing, entry)
		}
		result[eventType] = existing
	}

	return result
}
