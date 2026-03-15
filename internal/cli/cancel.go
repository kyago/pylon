package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/orchestrator"
	"github.com/kyago/pylon/internal/store"
)

func newCancelCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cancel [pipeline-id]",
		Short: "Cancel a running pipeline",
		Long: `Cancel a running pipeline and transition it to failed state.

Spec Reference: Section 7 "pylon cancel"`,
		Args: cobra.ExactArgs(1),
		RunE: runCancel,
	}
}

func runCancel(cmd *cobra.Command, args []string) error {
	pipelineID := args[0]

	startDir := flagWorkspace
	if startDir == "" {
		startDir = "."
	}
	root, err := config.FindWorkspaceRoot(startDir)
	if err != nil {
		return fmt.Errorf("not in a pylon workspace: %w", err)
	}

	cfg, err := config.LoadConfig(filepath.Join(root, ".pylon", "config.yml"))
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	dbPath := filepath.Join(root, ".pylon", "pylon.db")
	s, err := store.NewStore(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open store: %w", err)
	}
	defer s.Close()

	if err := s.Migrate(); err != nil {
		return fmt.Errorf("failed to migrate database: %w", err)
	}

	orch := orchestrator.NewOrchestrator(cfg, s, root)
	orch.SetPipelineID(pipelineID)

	// Recover current state
	if err := orch.Recover(); err != nil {
		return fmt.Errorf("recovery failed: %w", err)
	}

	if orch.Pipeline == nil || orch.Pipeline.ID != pipelineID {
		return fmt.Errorf("pipeline %s not found or not active", pipelineID)
	}

	// Transition to failed
	if err := orch.TransitionTo(orchestrator.StageFailed); err != nil {
		return fmt.Errorf("failed to cancel pipeline %s: %w", pipelineID, err)
	}

	fmt.Printf("✓ Pipeline %s cancelled\n", pipelineID)

	return nil
}
