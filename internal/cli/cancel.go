package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/orchestrator"
	"github.com/kyago/pylon/internal/store"
)

func newCancelCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cancel [pipeline-id]",
		Short: "Cancel a running pipeline",
		Long: `Cancel a running pipeline. Supports both v2 file-based and legacy SQLite pipelines.`,
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

	// v2: Try file-based cancellation first
	pipelineDir := filepath.Join(root, ".pylon", "runtime", pipelineID)
	if _, err := os.Stat(pipelineDir); err == nil {
		// Update status.json to cancelled
		statusData, _ := json.Marshal(map[string]string{
			"status":       "cancelled",
			"cancelled_at": time.Now().UTC().Format(time.RFC3339),
		})
		if err := os.WriteFile(filepath.Join(pipelineDir, "status.json"), statusData, 0644); err != nil {
			return fmt.Errorf("failed to update status: %w", err)
		}

		// Run cleanup script if available
		cleanupScript := filepath.Join(root, ".pylon", "scripts", "bash", "cleanup-pipeline.sh")
		if _, err := os.Stat(cleanupScript); err == nil {
			// Read status.json to get branch info
			var branch string
			if data, err := os.ReadFile(filepath.Join(pipelineDir, "status.json")); err == nil {
				var sj map[string]string
				if json.Unmarshal(data, &sj) == nil {
					branch = sj["branch"]
				}
			}
			cleanup := exec.Command("bash", cleanupScript, pipelineDir, branch)
			cleanup.Dir = root
			cleanup.Run() // best effort
		}

		fmt.Printf("✓ Pipeline %s cancelled\n", pipelineID)
		return nil
	}

	// Legacy: SQLite-based cancellation
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

	if _, err := orch.Recover(); err != nil {
		return fmt.Errorf("recovery failed: %w", err)
	}

	if orch.Pipeline == nil || orch.Pipeline.ID != pipelineID {
		return fmt.Errorf("pipeline %s not found", pipelineID)
	}

	if err := orch.TransitionTo(orchestrator.StageFailed); err != nil {
		return fmt.Errorf("failed to cancel pipeline %s: %w", pipelineID, err)
	}

	fmt.Printf("✓ Pipeline %s cancelled\n", pipelineID)
	return nil
}
