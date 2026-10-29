package cli

import (
	"io"

	"github.com/spf13/cobra"
)

// newSyncMemoryCmd keeps the removed `sync-memory` command as a hidden no-op: an old
// .claude/settings.json Stop hook (`pylon sync-memory --from-session --agent claude`)
// keeps calling it until launch or doctor rewrites the hooks, and an unknown command
// would fail the hook on every turn.
func newSyncMemoryCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "sync-memory",
		Short:  "(deprecated) 아무것도 하지 않음",
		Hidden: true,
		// 구버전 훅의 플래그(--from-session, --agent, --incremental 등)를 모두 받아들인다.
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, _ = io.Copy(io.Discard, cmd.InOrStdin()) // 훅이 넘긴 JSON을 소비만 한다
			return nil
		},
	}
}
