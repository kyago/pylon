package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/orchestrator"
	"github.com/kyago/pylon/internal/store"
	"github.com/kyago/pylon/internal/tmux"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current work status",
		Long: `Display the status of active tmux sessions, running tasks,
and queued work items.

Spec Reference: Section 7 "pylon status"`,
		RunE: runStatus,
	}
}

func runStatus(cmd *cobra.Command, args []string) error {
	// Find workspace
	startDir := flagWorkspace
	if startDir == "" {
		startDir = "."
	}
	root, err := config.FindWorkspaceRoot(startDir)
	if err != nil {
		return fmt.Errorf("not in a pylon workspace: %w", err)
	}

	// Load config for tmux prefix
	cfg, err := config.LoadConfig(root + "/.pylon/config.yml")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	mgr := tmux.NewManager(cfg.Tmux.SessionPrefix)
	sessions, err := mgr.List()
	if err != nil {
		return fmt.Errorf("failed to list tmux sessions: %w", err)
	}

	fmt.Println("Pylon Status")
	fmt.Println("─────────────────────────")

	if len(sessions) == 0 {
		fmt.Println("No active sessions.")
	} else {
		fmt.Println("Active Sessions:")
		for _, s := range sessions {
			ago := formatDuration(time.Since(s.Created))
			fmt.Printf("  ● %-20s alive     (created %s ago)\n", s.Name, ago)
		}
	}

	// Show pipeline status from store
	dbPath := root + "/.pylon/pylon.db"
	s, err := store.NewStore(dbPath)
	if err == nil {
		defer s.Close()
		s.Migrate()

		tmuxMgr := tmux.NewManager(cfg.Tmux.SessionPrefix)
		orch := orchestrator.NewOrchestrator(cfg, s, tmuxMgr, root)
		orch.Recover()

		fmt.Println()
		if orch.Pipeline != nil {
			fmt.Printf("Pipeline: %s\n", orch.Pipeline.ID)
			fmt.Printf("  Stage:    %s\n", orch.Pipeline.CurrentStage)
			fmt.Printf("  Attempts: %d/%d\n", orch.Pipeline.Attempts, orch.Pipeline.MaxAttempts)
			if len(orch.Pipeline.Agents) > 0 {
				fmt.Println("  Agents:")
				for name, agent := range orch.Pipeline.Agents {
					fmt.Printf("    %-15s %s\n", name, agent.Status)
				}
			}
		} else {
			fmt.Println("No active pipeline.")
		}
	} else {
		fmt.Println()
		fmt.Println("No active pipeline.")
	}

	return nil
}

// formatDuration returns a human-readable duration string.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}
