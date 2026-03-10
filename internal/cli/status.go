package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/orchestrator"
	"github.com/kyago/pylon/internal/store"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current work status",
		Long: `Display the status of running tasks and queued work items.

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

	// Load config
	cfg, err := config.LoadConfig(root + "/.pylon/config.yml")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	fmt.Println("Pylon Status")
	fmt.Println("─────────────────────────")

	// Show pipeline status from store
	dbPath := root + "/.pylon/pylon.db"
	s, err := store.NewStore(dbPath)
	if err == nil {
		defer s.Close()
		s.Migrate()

		orch := orchestrator.NewOrchestrator(cfg, s, root)
		orch.Recover()

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
		fmt.Println("No active pipeline.")
	}

	return nil
}
