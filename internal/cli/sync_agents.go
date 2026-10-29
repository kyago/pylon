package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newSyncAgentsCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
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

	cmd.Flags().BoolVar(&force, "force", false, "(deprecated) 효과 없음 — 내장 리소스는 항상 최신 버전으로 유지됩니다")
	_ = cmd.Flags().MarkDeprecated("force", "내장 리소스는 .pylon/builtin/에서 항상 최신 버전으로 유지됩니다")

	return cmd
}

func runSyncAgents() error {
	root, err := resolveRoot()
	if err != nil {
		return err
	}
	wrote, removed, err := syncBuiltinResources(root)
	if err != nil {
		return err
	}
	ensureGitignore(root)
	if err := syncClaudeAgentLinks(root); err != nil {
		fmt.Printf("⚠ .claude/agents/ 심링크 갱신 실패: %v\n", err)
	}
	reportBuiltinSync(wrote, removed, nil)
	return nil
}
