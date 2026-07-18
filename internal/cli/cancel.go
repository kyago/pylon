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
	"github.com/kyago/pylon/internal/history"
	"github.com/kyago/pylon/internal/store"
)

func newCancelCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cancel [pipeline-id]",
		Short: "Cancel a running pipeline",
		Long: `Cancel a running pipeline (file-based v2 pipelines under .pylon/runtime/).`,
		Args: cobra.ExactArgs(1),
		RunE: runCancel,
	}
}

func runCancel(cmd *cobra.Command, args []string) error {
	pipelineID := args[0]

	root, err := resolveRoot()
	if err != nil {
		return err
	}

	// v2: Try file-based cancellation first
	pipelineDir := filepath.Join(root, ".pylon", "runtime", pipelineID)
	if _, err := os.Stat(pipelineDir); err == nil {
		// Read existing status.json for branch info BEFORE overwriting
		var branch string
		if data, err := os.ReadFile(filepath.Join(pipelineDir, "status.json")); err == nil {
			var sj map[string]string
			if json.Unmarshal(data, &sj) == nil {
				branch = sj["branch"]
			}
		}

		// Update status.json to cancelled
		statusData, _ := json.Marshal(map[string]string{
			"status":       "cancelled",
			"cancelled_at": time.Now().UTC().Format(time.RFC3339),
		})
		if err := os.WriteFile(filepath.Join(pipelineDir, "status.json"), statusData, 0644); err != nil {
			return fmt.Errorf("failed to update status: %w", err)
		}

		// Record a final cancelled checkpoint so the runtime directory can be
		// deleted without losing history. Best effort: on failure we fall back
		// to preserving the runtime directory.
		checkpointed := false
		if cfg, cfgErr := config.LoadConfig(filepath.Join(root, ".pylon", "config.yml")); cfgErr == nil {
			if s, storeErr := store.NewStore(filepath.Join(root, ".pylon", "pylon.db")); storeErr == nil {
				if s.Migrate() == nil {
					mgr := history.NewManager(root, cfg.History, s, nil)
					if _, cpErr := mgr.Checkpoint(pipelineID, history.PhaseCancelled); cpErr == nil {
						checkpointed = true
					}
				}
				s.Close()
			}
		}

		// Run cleanup script if available
		cleanupScript := filepath.Join(root, ".pylon", "scripts", "bash", "cleanup-pipeline.sh")
		if _, err := os.Stat(cleanupScript); err == nil {
			cleanup := exec.Command("bash", cleanupScript, pipelineDir, branch, fmt.Sprintf("%t", checkpointed))
			cleanup.Dir = root
			cleanup.Run() // best effort
		}

		if checkpointed {
			fmt.Printf("✓ Pipeline %s cancelled (이력 체크포인트 기록됨)\n", pipelineID)
		} else {
			fmt.Printf("✓ Pipeline %s cancelled\n", pipelineID)
		}
		return nil
	}

	return fmt.Errorf("pipeline %s not found", pipelineID)
}
