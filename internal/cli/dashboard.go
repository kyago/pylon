package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newDashboardCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "dashboard",
		Short: "Start the web dashboard",
		Long: `Start a local web dashboard server (Templ + HTMX) for monitoring
agent status, task progress, and conversation history.

Spec Reference: Section 7 "pylon dashboard"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("not implemented yet (Phase 7)")
		},
	}
}
