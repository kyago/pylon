package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kyago/pylon/internal/config"
)

func newSyncAgentsCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "sync-agents",
		Short: "Sync built-in agent definitions to workspace",
		Long: `내장 에이전트 정의를 워크스페이스에 동기화합니다.

pylon에 내장된 최신 에이전트 템플릿(.pylon/agents/)을 설치하고
.claude/agents/ 심링크를 갱신합니다.

기존에 pylon을 설치한 사용자가 새 에이전트를 받거나,
에이전트 파일이 누락된 경우 사용합니다.

기본적으로 이미 존재하는 에이전트 파일은 건너뜁니다.
--force 를 사용하면 모든 에이전트를 최신 버전으로 덮어씁니다.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSyncAgents(force)
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "기존 에이전트 파일을 최신 버전으로 덮어쓰기")

	return cmd
}

func runSyncAgents(force bool) error {
	startDir := flagWorkspace
	if startDir == "" {
		startDir = "."
	}
	root, err := config.FindWorkspaceRoot(startDir)
	if err != nil {
		return fmt.Errorf("pylon 워크스페이스가 아닙니다 — 'pylon init'을 먼저 실행하세요")
	}

	pylonDir := filepath.Join(root, ".pylon")
	agentsDir := filepath.Join(pylonDir, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		return fmt.Errorf("agents 디렉토리 생성 실패: %w", err)
	}

	// 내장 에이전트 설치
	entries, err := embeddedAgents.ReadDir("agents")
	if err != nil {
		return fmt.Errorf("내장 에이전트 읽기 실패: %w", err)
	}

	var installed, skipped, updated int
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		agentPath := filepath.Join(agentsDir, entry.Name())

		_, statErr := os.Stat(agentPath)
		if statErr != nil && !os.IsNotExist(statErr) {
			fmt.Printf("⚠ %s 상태 확인 실패: %v\n", entry.Name(), statErr)
			skipped++
			continue
		}
		exists := statErr == nil

		if exists && !force {
			skipped++
			continue
		}

		content, err := embeddedAgents.ReadFile("agents/" + entry.Name())
		if err != nil {
			fmt.Printf("⚠ %s 읽기 실패: %v\n", entry.Name(), err)
			continue
		}

		if err := os.WriteFile(agentPath, content, 0644); err != nil {
			fmt.Printf("⚠ %s 쓰기 실패: %v\n", entry.Name(), err)
			continue
		}

		if exists {
			updated++
		} else {
			installed++
		}
	}

	// .claude/agents/ 심링크 갱신
	if err := syncClaudeAgentLinks(root, pylonDir); err != nil {
		fmt.Printf("⚠ .claude/agents/ 심링크 갱신 실패: %v\n", err)
	}

	fmt.Printf("에이전트 동기화 완료: %d개 신규, %d개 갱신, %d개 스킵\n", installed, updated, skipped)
	return nil
}

// syncClaudeAgentLinks ensures .claude/agents/ has symlinks for all .pylon/agents/*.md files.
func syncClaudeAgentLinks(workDir, pylonDir string) error {
	claudeAgentsDir := filepath.Join(workDir, ".claude", "agents")
	if err := os.MkdirAll(claudeAgentsDir, 0755); err != nil {
		return err
	}

	pylonAgentsDir := filepath.Join(pylonDir, "agents")
	entries, err := os.ReadDir(pylonAgentsDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		relTarget := filepath.Join("..", "..", ".pylon", "agents", entry.Name())
		linkPath := filepath.Join(claudeAgentsDir, entry.Name())

		// 기존 심링크가 있으면 제거 후 재생성 (대상이 변경됐을 수 있음)
		if info, err := os.Lstat(linkPath); err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				if err := os.Remove(linkPath); err != nil {
					fmt.Printf("⚠ 심링크 제거 실패 %s: %v\n", entry.Name(), err)
					continue
				}
			} else {
				continue // 일반 파일이면 건드리지 않음
			}
		}

		if err := os.Symlink(relTarget, linkPath); err != nil && !os.IsExist(err) {
			fmt.Printf("⚠ 심링크 생성 실패 %s: %v\n", entry.Name(), err)
		}
	}

	return nil
}
