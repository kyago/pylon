package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newSyncAgentsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync-agents",
		Short: "Sync built-in agent definitions to workspace",
		Long: `내장 에이전트 정의를 워크스페이스에 동기화합니다.

pylon에 내장된 최신 리소스(.pylon/builtin/)를 갱신하고
.claude/agents/ 심링크를 갱신합니다. launch와 pylon doctor도 같은 동기화를
수행하므로 보통은 직접 실행할 필요가 없습니다.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSyncAgents()
		},
	}

}

func runSyncAgents() error {
	root, err := resolveRoot()
	if err != nil {
		return err
	}
	wrote, err := syncBuiltinResources(root)
	if err != nil {
		return err
	}
	ensureGitignore(root)
	if err := syncClaudeAgentLinks(root); err != nil {
		fmt.Printf("⚠ .claude/agents/ 심링크 갱신 실패: %v\n", err)
	}
	reportBuiltinSync(wrote, nil)
	return nil
}
