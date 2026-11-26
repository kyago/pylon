package cli

import (
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/layout"
)

//go:embed reference/*.md
var embeddedReference embed.FS

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "현재 디렉터리를 pylon 워크스페이스로 초기화",
		Long: `현재 디렉터리를 pylon 워크스페이스로 초기화합니다.

.pylon/(설정, 도메인 지식, 내장 커맨드, 메모리)과 Claude Code용 생성 파일,
루트 AGENTS.md·CLAUDE.md를 만듭니다.`,
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
		return fmt.Errorf("도구 검사 실패: %w", err)
	}
	if !passed {
		return fmt.Errorf("필수 도구가 없습니다. 설치한 뒤 'pylon init'을 다시 실행하세요")
	}
	fmt.Println("모든 검사를 통과했습니다.")
	fmt.Println()

	// Determine workspace directory
	workDir := flagWorkspace
	if workDir == "" {
		workDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("작업 디렉터리 확인 실패: %w", err)
		}
	}

	// Step 2: Check if .pylon/ already exists
	pylonDir := layout.PylonDir(workDir)
	if _, err := os.Stat(pylonDir); err == nil {
		return fmt.Errorf("%s에 .pylon/이 이미 있습니다. 먼저 'pylon uninstall'로 제거하세요", workDir)
	}

	// Step 3: Interactive input
	fmt.Println("Pylon 워크스페이스 초기화")
	fmt.Println(strings.Repeat("\u2500", 40))

	// Step 4: Create directory structure
	dirs := []string{
		filepath.Join(pylonDir, "domain"),
		filepath.Join(pylonDir, "agents"),
		filepath.Join(pylonDir, "skills"),
		filepath.Join(pylonDir, "commands"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("디렉터리 %s 생성 실패: %w", dir, err)
		}
	}

	// Create config.yml (Spec Section 16: minimal config after init)
	configContent := "version: \"0.2\"\n" +
		"# true면 PR을 만든 뒤 서브 에이전트 리뷰를 돌리라는 지침을 AGENTS.md에 넣습니다. 바꾼 뒤 pylon doctor를 실행하세요.\n" +
		"auto_review: false\n"

	if err := os.WriteFile(filepath.Join(pylonDir, "config.yml"), []byte(configContent), 0644); err != nil {
		return fmt.Errorf("config.yml 생성 실패: %w", err)
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
			return fmt.Errorf("%s 생성 실패: %w", name, err)
		}
	}

	// Built-in commands and the usage manual (.pylon/builtin/)
	if _, err := syncBuiltinResources(workDir); err != nil {
		return err
	}

	// .claude/commands/ so /pl:* works right after init, without a doctor run
	if err := applyClaudeCommands(layout.ClaudeCommandsDir(workDir), buildDesiredClaudeCommands(workDir)); err != nil {
		return err
	}

	// Create symlinks in .claude/agents/ for Claude CLI native discovery
	if err := syncClaudeAgentLinks(workDir); err != nil {
		return err
	}
	if err := syncClaudeSkillLinks(workDir); err != nil {
		return err
	}

	// Step 5: Keep generated files out of git when the root is in a repository.
	// .pylon/memory/ is not in the list, so such a root can track it.
	if err := ensureGitignore(workDir); err != nil {
		return fmt.Errorf(".gitignore 갱신 실패: %w", err)
	}

	if err := os.MkdirAll(layout.MemoryDir(workDir), 0755); err != nil {
		return fmt.Errorf("memory 디렉토리 생성 실패: %w", err)
	}

	// Step 6: Discover projects and ensure .pylon/ is git-excluded per project
	projects, err := config.DiscoverProjects(workDir)
	if err != nil {
		fmt.Printf("⚠ 프로젝트 탐색 실패: %v\n", err)
	}
	for _, p := range projects {
		// Ensure .pylon/ is excluded from project git tracking (skip non-git dirs)
		if err := excludePylonFromRepo(p.Path); err != nil {
			if !errors.Is(err, errNotGitRepo) {
				fmt.Printf("⚠ %s: .pylon/ exclude 설정 실패: %v\n", p.Name, err)
			}
		}
	}

	// Root agent files so the workspace is ready: CLAUDE.md import marker +
	// AGENTS.md whose pylon block carries the embedded manual and workspace facts.
	_, backedUp, err := ensureRootAgentFiles(workDir, projects)
	if err != nil {
		return err
	}
	for _, name := range backedUp {
		fmt.Printf("ℹ 기존 내용을 %s에 백업했습니다 — pylon이 이 파일을 관리합니다.\n", name)
	}

	fmt.Println()
	fmt.Printf("Pylon 워크스페이스를 초기화했습니다: %s\n", workDir)
	fmt.Println()
	fmt.Println("생성한 항목:")
	fmt.Println("  .pylon/config.yml          - 워크스페이스 표시와 설정(auto_review)")
	fmt.Println("  .pylon/domain/             - 도메인 지식")
	fmt.Println("  .pylon/builtin/            - 내장 커맨드·매뉴얼 (doctor가 다시 씀)")
	fmt.Println("  .pylon/agents/             - 사용자 에이전트")
	fmt.Println("  .pylon/skills/             - 사용자 스킬")
	fmt.Println("  .pylon/commands/           - 사용자 슬래시 커맨드")
	fmt.Println("  .pylon/memory/             - 프로젝트 메모리")
	fmt.Println("  .claude/                   - Claude Code용 슬래시 커맨드와 사용자 에이전트·스킬 심링크 (doctor가 다시 씀)")
	fmt.Println("  CLAUDE.md                  - @AGENTS.md import 마커")
	fmt.Println("  AGENTS.md                  - 운영 가이드 (pylon 블록은 doctor가 다시 씀, 워크스페이스 규칙은 블록 밖에 작성)")
	fmt.Println()
	fmt.Println("다음 단계:")
	fmt.Println("  1. 프로젝트 추가: pylon add-project <git-url>")
	fmt.Println("  2. 작업 시작: 이 디렉터리에서 Claude Code, Codex, Paseo 세션을 엽니다")

	return nil
}
