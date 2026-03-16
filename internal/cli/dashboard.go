package cli

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/kyago/pylon/internal/dashboard"
)

func newDashboardCmd() *cobra.Command {
	var port int

	cmd := &cobra.Command{
		Use:   "dashboard",
		Short: "Start the web dashboard",
		Long: `Start a local web dashboard server (HTMX + SSE) for monitoring
pipeline status, agent activity, and message queue state.

Spec Reference: Section 7 "pylon dashboard"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, cfg, s, err := openWorkspaceStore()
			if err != nil {
				return err
			}
			defer s.Close()

			dashCfg := cfg.Dashboard
			if port > 0 {
				dashCfg.Port = port
			}

			srv, err := dashboard.NewServer(s, &dashCfg, &cfg.Runtime)
			if err != nil {
				return fmt.Errorf("failed to create dashboard server: %w", err)
			}

			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			return srv.Start(ctx)
		},
	}

	cmd.Flags().IntVar(&port, "port", 0, "override dashboard port (default from config)")

	return cmd
}
