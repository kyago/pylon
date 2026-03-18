package cli

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
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
			root, cfg, s, err := openWorkspaceStore()
			if err != nil {
				return err
			}
			defer s.Close()

			// Check if dashboard already running for this workspace
			if existing := checkExistingDashboard(root); existing != nil {
				return fmt.Errorf("대시보드가 이미 실행 중입니다: http://%s:%d", existing.Host, existing.Port)
			}

			dashCfg := cfg.Dashboard
			if port > 0 {
				dashCfg.Port = port
			}

			wsName := filepath.Base(root)

			// Standalone dashboard: log to stderr (no TUI conflict)
			logger := log.New(os.Stderr, "dashboard: ", log.LstdFlags)

			srv, err := dashboard.NewServer(s, &dashCfg, wsName, logger)
			if err != nil {
				return fmt.Errorf("failed to create dashboard server: %w", err)
			}

			// Write dashboard info for discovery by other pylon instances
			ln, err := srv.Listen()
			if err != nil {
				return fmt.Errorf("대시보드 포트 바인딩 실패: %w", err)
			}
			actualPort := ln.Addr().(*net.TCPAddr).Port

			if err := writeDashboardInfo(root, dashCfg.Host, actualPort); err != nil {
				fmt.Fprintf(os.Stderr, "⚠ 대시보드 정보 기록 실패: %v\n", err)
			}
			defer removeDashboardInfo(root)

			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			fmt.Printf("📊 대시보드: http://%s:%d (%s)\n", cfg.Dashboard.Host, actualPort, wsName)
			return srv.Serve(ctx, ln)
		},
	}

	cmd.Flags().IntVar(&port, "port", 0, "override dashboard port (default from config)")

	return cmd
}
