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

	// Step 4: Load project memory for context injection
	var memoryContext string
	dbPath := filepath.Join(root, ".pylon", "pylon.db")
	if _, err := os.Stat(dbPath); err == nil {
		s, err := store.NewStore(dbPath)
		if err == nil {
			_ = s.Migrate()
			memoryContext = buildMemoryContext(s, projects)
			s.Close()
		}
	}

	// Step 5: Generate .claude/ directory structure
	if err := generateClaudeDir(root, cfg, projects, memoryContext); err != nil {
		return fmt.Errorf(".claude/ 생성 실패: %w", err)
	}

	// Ensure .claude/ and CLAUDE.md are in .gitignore
	ensureGitignore(root)

	// Step 6: Select permission mode
	permMode, err := selectPermissionMode(cfg.Runtime.PermissionMode)
	if err != nil {
		return err
	}

	// Step 7: Launch Claude Code (replace process)
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
func generateClaudeDir(root string, cfg *config.Config, projects []config.ProjectInfo, memoryContext string) error {
	claudeDir := filepath.Join(root, ".claude")
	commandsDir := filepath.Join(claudeDir, "commands")

	// Ensure directories exist
	if err := os.MkdirAll(commandsDir, 0755); err != nil {
		return err
	}

	// Generate CLAUDE.md at workspace root
	claudeMD := buildRootCLAUDEMD(cfg, projects, memoryContext, root)
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
					continue
				}
				_ = os.WriteFile(destPath, content, 0755)
			}
		}
	}

	// Generate hooks in .claude/settings.json for Claude Code session lifecycle
	if err := generateSettingsHooks(claudeDir); err != nil {
		return fmt.Errorf("settings.json hooks 생성 실패: %w", err)
	}

	return nil
}

// buildRootCLAUDEMD generates the root agent system prompt.
func buildRootCLAUDEMD(cfg *config.Config, projects []config.ProjectInfo, memoryContext string, root string) string {
	var b strings.Builder

	// Identity
	b.WriteString("# Pylon — AI 개발팀 오케스트레이터\n\n")
	b.WriteString("당신은 Pylon의 루트 에이전트(PO)입니다.\n")
	b.WriteString("사용자의 요구사항을 분석하고, AI 에이전트 팀을 오케스트레이션하여\n")
	b.WriteString("분석 → 설계 → 구현 → 검증 → PR 생성까지 자동 수행합니다.\n\n")

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

	// Pipeline stages
	b.WriteString("## 파이프라인 단계\n\n")
	b.WriteString("요구사항 처리 시 다음 단계를 순서대로 수행합니다:\n\n")
	b.WriteString("1. **PO 대화** — 요구사항 분석, 역질문, 수용 기준 확정\n")
	b.WriteString("2. **Architect 분석** — 기술 방향성, 의존성, 아키텍처 결정\n")
	b.WriteString("3. **PM 태스크 분해** — 작업 분해, 에이전트 할당, 실행 순서\n")
	b.WriteString("4. **에이전트 실행** — 프로젝트별 에이전트가 병렬 구현\n")
	b.WriteString("5. **교차 검증** — 빌드/테스트/린트 자동 검증\n")
	b.WriteString("6. **PR 생성** — 변경사항 PR 생성\n")
	b.WriteString("7. **PO 검증** — 수용 기준 충족 확인\n")
	b.WriteString("8. **위키 갱신** — 도메인 지식 자동 업데이트\n\n")

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
	b.WriteString("- `/pl:cancel` — 파이프라인 취소\n")
	b.WriteString("- `/pl:index` — 프로젝트 코드베이스 인덱싱\n\n")

	// Sub-agent orchestration
	b.WriteString("## 서브 에이전트 오케스트레이션\n\n")
	b.WriteString("Claude Code의 Agent 도구를 사용하여 서브 에이전트를 병렬 실행합니다.\n")
	b.WriteString("각 에이전트 정의는 `.pylon/agents/`에 있습니다.\n")
	b.WriteString("독립 태스크는 단일 메시지에서 여러 Agent 호출로 병렬 실행합니다.\n")
	b.WriteString("`isolation: \"worktree\"` 옵션으로 git worktree 격리를 사용합니다.\n\n")

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

	// Memory context (pre-injected from SQLite)
	if memoryContext != "" {
		b.WriteString("## 프로젝트 메모리 요약\n\n")
		b.WriteString(memoryContext)
		b.WriteString("\n")
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

// buildSlashCommands generates .claude/commands/pl/ markdown files.
func buildSlashCommands(root string) map[string]string {
	commands := map[string]string{
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

// buildMemoryContext reads project memory from SQLite and formats it for CLAUDE.md injection.
func buildMemoryContext(s *store.Store, projects []config.ProjectInfo) string {
	if len(projects) == 0 {
		return ""
	}

	var b strings.Builder
	for _, p := range projects {
		entries, err := s.GetMemoryByCategory(p.Name, "codebase")
		if err != nil || len(entries) == 0 {
			continue
		}

		b.WriteString(fmt.Sprintf("### %s\n", p.Name))
		for _, e := range entries {
			b.WriteString(fmt.Sprintf("- **%s**: %s\n", e.Key, e.Content))
		}
		b.WriteString("\n")
	}

	return b.String()
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
