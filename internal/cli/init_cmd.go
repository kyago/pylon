package cli

import (
	"bufio"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/layout"
)

//go:embed agents/*.md
var embeddedAgents embed.FS

//go:embed scripts/bash/*.sh
var embeddedScripts embed.FS

//go:embed skills/*.md
var embeddedSkills embed.FS

//go:embed reference/*.md
var embeddedReference embed.FS

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize a new pylon workspace",
		Long: `Initialize the current directory as a pylon workspace.

Creates the .pylon/ directory structure with default configuration,
agent definitions, and runtime directories.

Spec Reference: Section 7 "pylon init"`,
		RunE: runInit,
	}
}

// initDoctorChecks is the required-tool gate for `pylon init`, indirected so tests
// can exercise initialization on machines without git/gh/claude installed — CI
// runners have no claude binary, which previously failed the init tests.
var initDoctorChecks = RunDoctorChecks

func runInit(cmd *cobra.Command, args []string) error {
	// Step 1: Run doctor checks
	passed, err := initDoctorChecks()
	if err != nil {
		return fmt.Errorf("doctor check failed: %w", err)
	}
	if !passed {
		return fmt.Errorf("required tools are missing — install them and retry 'pylon init'")
	}
	fmt.Println("All checks passed.")
	fmt.Println()

	// Determine workspace directory
	workDir := flagWorkspace
	if workDir == "" {
		workDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get working directory: %w", err)
		}
	}

	// Step 2: Check if .pylon/ already exists
	pylonDir := layout.PylonDir(workDir)
	if _, err := os.Stat(pylonDir); err == nil {
		return fmt.Errorf(".pylon/ already exists in %s. Use 'pylon uninstall' to remove it first", workDir)
	}

	// Step 3: Interactive input
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("Pylon Workspace Initialization")
	fmt.Println(strings.Repeat("\u2500", 40))

	providerInput := "auto"
	fmt.Printf("Runtime provider: %s\n", providerInput)

	// PR reviewer
	fmt.Printf("PR reviewer GitHub username (Enter to skip): ")
	reviewerInput, _ := reader.ReadString('\n')
	reviewerInput = strings.TrimSpace(reviewerInput)

	// Step 4: Create directory structure
	// Spec Reference: Section 4 "Workspace Structure", Section 7 "pylon init"
	dirs := []string{
		filepath.Join(pylonDir, "domain"),
		filepath.Join(pylonDir, "agents"),
		filepath.Join(pylonDir, "skills"),
		filepath.Join(pylonDir, "commands"),
		filepath.Join(pylonDir, "runtime", "memory"),
		filepath.Join(pylonDir, "runtime", "sessions"),
		filepath.Join(pylonDir, "conversations"),
		filepath.Join(pylonDir, "tasks"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Create config.yml (Spec Section 16: minimal config after init)
	reviewerSection := "    reviewers: []"
	if reviewerInput != "" {
		reviewerSection = fmt.Sprintf("    reviewers:\n      - %s", reviewerInput)
	}

	configContent := fmt.Sprintf(`version: "0.2"

runtime:
  provider: %s
  execution_mode: session-native
  permission_mode: acceptEdits

git:
  pr:
%s
`, providerInput, reviewerSection)

	if err := os.WriteFile(filepath.Join(pylonDir, "config.yml"), []byte(configContent), 0644); err != nil {
		return fmt.Errorf("failed to create config.yml: %w", err)
	}

	// Create domain knowledge templates (domain-neutral for multi-domain support)
	domainFiles := map[string]string{
		"practices.md": "# 작업 관행\n\n> 이 파일은 AI 에이전트가 관리합니다. 수정이 필요하면 루트 에이전트에게 요청하세요.\n\n## 핵심 원칙\n\n## 품질 기준\n\n## 커뮤니케이션 규칙\n",
		"overview.md":  "# 프로젝트 개요\n\n> 이 파일은 AI 에이전트가 관리합니다. 수정이 필요하면 루트 에이전트에게 요청하세요.\n\n## 목표\n\n## 구조\n\n## 주요 결정 사항\n",
		"glossary.md":  "# 용어 사전\n\n> 이 파일은 AI 에이전트가 관리합니다. 수정이 필요하면 루트 에이전트에게 요청하세요.\n\n| 용어 | 정의 |\n|------|------|\n",
	}

	for name, content := range domainFiles {
		path := filepath.Join(pylonDir, "domain", name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to create %s: %w", name, err)
		}
	}

	// Built-in agents, skills, commands, scripts and the usage manual (.pylon/builtin/)
	if _, _, err := syncBuiltinResources(workDir); err != nil {
		return err
	}

	// Create symlinks in .claude/agents/ for Claude CLI native discovery
	if err := syncClaudeAgentLinks(workDir); err != nil {
		return err
	}

	// Step 5: Update .gitignore
	gitignorePath := filepath.Join(workDir, ".gitignore")
	gitignoreEntries := []string{
		"# Pylon runtime (agent communication, state)",
		".pylon/runtime/",
		".pylon/conversations/",
		".pylon/history/",
		// .pylon/memory/ 는 의도적으로 제외 — 메모리는 git으로 추적/리뷰한다 (D1).
		"",
		"# Claude CLI agent symlinks (managed by pylon)",
		".claude/agents/",
		"",
		"# Pylon built-in resources (regenerated)",
		".pylon/builtin/",
		"",
		"# Pylon root agent files (regenerated)",
		"CLAUDE.md",
		"AGENTS.md",
		"",
	}
	gitignoreContent := strings.Join(gitignoreEntries, "\n")

	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to update .gitignore: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString("\n" + gitignoreContent); err != nil {
		return fmt.Errorf("failed to write .gitignore: %w", err)
	}

	// 메모리 저장소 루트를 미리 만들어 git 추적을 시작한다 (D1).
	if err := os.MkdirAll(layout.MemoryDir(workDir), 0755); err != nil {
		return fmt.Errorf("memory 디렉토리 생성 실패: %w", err)
	}
	if err := os.WriteFile(filepath.Join(layout.MemoryDir(workDir), ".gitkeep"), nil, 0644); err != nil {
		return fmt.Errorf(".gitkeep 생성 실패: %w", err)
	}
	if _, err := os.Stat(filepath.Join(workDir, ".git")); os.IsNotExist(err) {
		fmt.Println("ℹ 워크스페이스가 git 저장소가 아닙니다 — 프로젝트 메모리(.pylon/memory/)의 버전 관리를 위해 'git init'을 권장합니다")
	}

	// Step 6: Discover projects and ensure .pylon/ is git-excluded per project
	projects, err := config.DiscoverProjects(workDir)
	if err != nil {
		fmt.Printf("⚠ 프로젝트 탐색 실패: %v\n", err)
	}
	for _, p := range projects {
		// Ensure .pylon/ is excluded from project git tracking (skip non-git dirs)
		if err := excludePylonFromRepo(p.Path); err != nil {
			if !strings.Contains(err.Error(), "not a git repository") {
				fmt.Printf("⚠ %s: .pylon/ exclude 설정 실패: %v\n", p.Name, err)
			}
		}
	}

	// Root agent files so the workspace is launch-ready: CLAUDE.md import marker +
	// AGENTS.md whose pylon block carries the embedded manual and workspace facts.
	_, backedUp, err := ensureRootAgentFiles(workDir, projects)
	if err != nil {
		return err
	}
	for _, name := range backedUp {
		fmt.Printf("ℹ 기존 내용을 %s에 백업했습니다 — pylon이 이 파일을 관리합니다.\n", name)
	}

	fmt.Println()
	fmt.Printf("Pylon workspace initialized at %s\n", workDir)
	fmt.Println()
	fmt.Println("Created:")
	fmt.Println("  .pylon/config.yml          - workspace configuration")
	fmt.Println("  .pylon/domain/             - team domain knowledge (wiki)")
	fmt.Println("  .pylon/builtin/            - built-in agents, skills, commands, scripts, manual (regenerated)")
	fmt.Println("  .pylon/agents/             - your own agents (pylon add-agent)")
	fmt.Println("  .pylon/skills/             - your own skills (pylon add-skill)")
	fmt.Println("  .pylon/commands/           - your own pipeline commands")
	fmt.Println("  .pylon/runtime/            - agent communication runtime")
	fmt.Println("  .pylon/history/            - pipeline work history (file-based)")
	fmt.Println("  .pylon/memory/             - project memory (git-tracked)")
	fmt.Println("  .pylon/conversations/      - conversation history")
	fmt.Println("  .pylon/tasks/              - confirmed task specs")
	fmt.Println("  .claude/agents/            - Claude CLI symlinks (-> .pylon/builtin/agents/, .pylon/agents/)")
	fmt.Println("  CLAUDE.md                  - @AGENTS.md import marker")
	fmt.Println("  AGENTS.md                  - operating guide (pylon block regenerated; write your own rules outside it)")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Edit .pylon/config.yml to customize settings")
	fmt.Println("  2. Add projects: pylon add-project <name>")
	fmt.Println("  3. Start working: run 'pylon', then use /pl:pipeline")

	return nil
}
