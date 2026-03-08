package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kyago/pylon/internal/agent"
	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/store"
	"github.com/kyago/pylon/internal/tmux"
)

func newIndexCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "index",
		Short: "Index project codebases for agent context",
		Long: `Analyze project codebases and update domain wiki and project context.

Launches a Tech Writer agent in a tmux session to scan selected projects,
update domain knowledge documents, and refresh project context.

Spec Reference: Section 12 "Domain Knowledge"`,
		Args: cobra.NoArgs,
		RunE: runIndex,
	}
}

// selectProjects presents an interactive menu for the user to choose
// which projects to index. Returns the selected subset.
func selectProjects(projects []config.ProjectInfo) ([]config.ProjectInfo, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("프로젝트를 선택하세요:")
	fmt.Println()
	fmt.Printf("  [0] 전체 인덱싱 (모든 프로젝트)\n")
	for i, p := range projects {
		fmt.Printf("  [%d] %s\n", i+1, p.Name)
	}
	fmt.Println()
	fmt.Printf("번호 입력 (기본값: 0): ")

	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(answer)

	// Default: all projects
	if answer == "" || answer == "0" {
		return projects, nil
	}

	num, err := strconv.Atoi(answer)
	if err != nil || num < 1 || num > len(projects) {
		return nil, fmt.Errorf("유효하지 않은 번호: %s", answer)
	}

	return []config.ProjectInfo{projects[num-1]}, nil
}

func runIndex(cmd *cobra.Command, args []string) error {
	// Step 1: Find workspace root
	startDir := flagWorkspace
	if startDir == "" {
		startDir = "."
	}
	root, err := config.FindWorkspaceRoot(startDir)
	if err != nil {
		return fmt.Errorf("not in a pylon workspace — run 'pylon init' first")
	}

	// Step 2: Load config
	cfg, err := config.LoadConfig(filepath.Join(root, ".pylon", "config.yml"))
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Step 3: Discover projects
	projects, err := config.DiscoverProjects(root)
	if err != nil {
		return fmt.Errorf("failed to discover projects: %w", err)
	}
	if len(projects) == 0 {
		return fmt.Errorf("프로젝트가 없습니다 — 'pylon add-project'로 프로젝트를 추가하세요")
	}

	// Step 4: Interactive project selection
	selected, err := selectProjects(projects)
	if err != nil {
		return err
	}

	fmt.Printf("\n인덱싱 대상: %d개 프로젝트\n", len(selected))
	for _, p := range selected {
		fmt.Printf("  - %s\n", p.Name)
	}

	// Step 5: Load or create tech-writer agent config
	agentCfg, err := loadTechWriterAgent(root, cfg)
	if err != nil {
		return fmt.Errorf("failed to load tech-writer agent: %w", err)
	}

	// Step 6: Open store, migrate, and seed project memory
	dbPath := filepath.Join(root, ".pylon", "pylon.db")
	s, err := store.NewStore(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open store: %w", err)
	}
	defer s.Close()

	if err := s.Migrate(); err != nil {
		return fmt.Errorf("failed to migrate database: %w", err)
	}

	for _, p := range selected {
		seedProjectMemory(s, p)
	}

	// Step 7: Build dynamic CLAUDE.md
	taskPrompt := buildIndexTaskPrompt(selected, root)

	builder := &agent.ClaudeMDBuilder{}
	claudeMD, err := builder.Build(agent.BuildInput{
		CommunicationRules: agent.DefaultCommunicationRules(),
		TaskContext:        taskPrompt,
		CompactionRules:    agent.DefaultCompactionRules(),
	})
	if err != nil {
		return fmt.Errorf("failed to build CLAUDE.md: %w", err)
	}

	// Step 8: Launch agent in tmux
	tmuxMgr := tmux.NewManager(cfg.Tmux.SessionPrefix)
	runner := agent.NewRunner(tmuxMgr)

	runCfg := agent.RunConfig{
		Agent:      agentCfg,
		Global:     cfg,
		TaskPrompt: taskPrompt,
		WorkDir:    root,
		ClaudeMD:   claudeMD,
	}

	if err := runner.Start(runCfg); err != nil {
		return fmt.Errorf("failed to start tech-writer agent: %w", err)
	}

	// Step 9: Output monitoring instructions
	prefix := cfg.Tmux.SessionPrefix
	if prefix == "" {
		prefix = "pylon"
	}
	sessionName := prefix + "-" + agentCfg.Name

	fmt.Println()
	fmt.Println("🚀 Tech Writer 에이전트가 시작되었습니다.")
	fmt.Println()
	fmt.Printf("  모니터링: tmux attach -t %s\n", sessionName)
	fmt.Printf("  상태 확인: pylon status\n")
	fmt.Printf("  중단:     pylon cancel\n")

	return nil
}

// loadTechWriterAgent loads the tech-writer agent from the workspace's
// .pylon/agents/tech-writer.md, falling back to a sensible default config
// if the file doesn't exist.
func loadTechWriterAgent(root string, cfg *config.Config) (*config.AgentConfig, error) {
	agentPath := filepath.Join(root, ".pylon", "agents", "tech-writer.md")

	agentCfg, err := config.ParseAgentFile(agentPath)
	if err == nil {
		agentCfg.ResolveDefaults(cfg)
		return agentCfg, nil
	}

	// File doesn't exist or is invalid — use default config
	agentCfg = &config.AgentConfig{
		Name:           "tech-writer",
		Role:           "Tech Writer",
		Backend:        "claude-code",
		MaxTurns:       100,
		PermissionMode: "bypassPermissions",
		Model:          "sonnet",
	}
	agentCfg.ResolveDefaults(cfg)
	return agentCfg, nil
}

// buildIndexTaskPrompt creates the task prompt that instructs the tech-writer
// agent on which projects to index and what to produce.
func buildIndexTaskPrompt(projects []config.ProjectInfo, root string) string {
	var b strings.Builder

	b.WriteString("# 코드베이스 인덱싱 태스크\n\n")

	b.WriteString("## 대상 프로젝트\n")
	for _, p := range projects {
		relPath, err := filepath.Rel(root, p.Path)
		if err != nil {
			relPath = p.Path
		}
		b.WriteString(fmt.Sprintf("- **%s**: `%s`\n", p.Name, relPath))
	}

	b.WriteString("\n## 수행할 작업\n\n")
	b.WriteString("각 프로젝트에 대해 다음을 수행하세요:\n\n")
	b.WriteString("1. **코드베이스 스캔**: 디렉토리 구조, 주요 파일, 의존성을 분석\n")
	b.WriteString("2. **도메인 위키 갱신**: `.pylon/wiki/` 하위에 도메인 지식 문서를 생성/갱신\n")
	b.WriteString("   - 아키텍처 개요\n")
	b.WriteString("   - 주요 모듈/패키지 설명\n")
	b.WriteString("   - 데이터 모델 및 API 엔드포인트\n")
	b.WriteString("   - 기술 스택 및 빌드/테스트 방법\n")
	b.WriteString("3. **프로젝트 컨텍스트 갱신**: `{프로젝트}/.pylon/context.md`를 실제 코드에 맞게 갱신\n\n")

	b.WriteString("## 주의사항\n\n")
	b.WriteString("- 기존 내용이 있으면 보존하되, 변경된 부분만 업데이트\n")
	b.WriteString("- 추측이 아닌 코드에서 확인된 사실만 기록\n")
	b.WriteString("- 위키 문서는 마크다운 형식으로 작성\n")
	b.WriteString("- 작업 완료 후 결과를 outbox에 기록\n")

	return b.String()
}

// seedProjectMemory inserts basic project metadata into SQLite
// so the tech-writer agent has initial context. Duplicate errors
// are logged as warnings, not fatal.
func seedProjectMemory(s *store.Store, project config.ProjectInfo) {
	stack := detectTechStack(project.Path)

	entries := []store.MemoryEntry{
		{
			ProjectID:  project.Name,
			Category:   "codebase",
			Key:        "tech-stack",
			Content:    fmt.Sprintf("Language: %s, Framework: %s, Build: %s", stack.Language, stack.Framework, stack.BuildTool),
			Author:     "pylon-index",
			Confidence: 0.9,
		},
		{
			ProjectID:  project.Name,
			Category:   "codebase",
			Key:        "project-path",
			Content:    project.Path,
			Author:     "pylon-index",
			Confidence: 1.0,
		},
	}

	for i := range entries {
		if err := s.InsertMemory(&entries[i]); err != nil {
			fmt.Printf("  ⚠ 메모리 저장 경고 (%s/%s): %v\n", project.Name, entries[i].Key, err)
		}
	}
}
