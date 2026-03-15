package cli

import (
	"fmt"
	"path/filepath"

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

	fmt.Println("Pylon Status")
	fmt.Println("─────────────────────────")

	// Show pipeline status from store
	dbPath := filepath.Join(root, ".pylon", "pylon.db")
	s, err := store.NewStore(dbPath)
	if err == nil {
		defer s.Close()
		s.Migrate()

		actives, listErr := s.GetActivePipelines()
		if listErr != nil {
			fmt.Printf("⚠ Failed to query pipelines: %v\n", listErr)
		} else if len(actives) == 0 {
			fmt.Println("No active pipeline.")
		} else {
			for i, rec := range actives {
				pipeline, pErr := orchestrator.LoadPipeline([]byte(rec.StateJSON))
				if pErr != nil {
					fmt.Printf("Pipeline: %s (state parse error: %v)\n", rec.PipelineID, pErr)
					continue
				}
				if i > 0 {
					fmt.Println()
				}
				fmt.Printf("Pipeline: %s\n", pipeline.ID)
				fmt.Printf("  Stage:    %s\n", pipeline.CurrentStage)
				fmt.Printf("  Attempts: %d/%d\n", pipeline.Attempts, pipeline.MaxAttempts)
				if len(pipeline.Agents) > 0 {
					fmt.Println("  Agents:")
					for name, agent := range pipeline.Agents {
						fmt.Printf("    %-15s %s\n", name, agent.Status)
					}
				}
			}
		}
	} else {
		fmt.Println("No active pipeline.")
	}

	return nil
}
