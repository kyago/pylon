package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kyago/pylon/internal/layout"
	"github.com/spf13/cobra"
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
	root, err := resolveRoot()
	if err != nil {
		return err
	}

	pylonDir := layout.PylonDir(root)
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
