package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"
	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/domain"
	"github.com/kyago/pylon/internal/orchestrator"
	"github.com/kyago/pylon/internal/store"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current work status",
		Long: `Display the status of running tasks and queued work items.

Shows both file-based v2 pipelines (.pylon/runtime/) and legacy SQLite pipelines.`,
		RunE: runStatus,
	}
}

func runStatus(cmd *cobra.Command, args []string) error {
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

	foundAny := false

	// v2: Show file-based pipeline status
	runtimeDir := filepath.Join(root, ".pylon", "runtime")
	if entries, dirErr := os.ReadDir(runtimeDir); dirErr == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			pipelineDir := filepath.Join(runtimeDir, entry.Name())
			if _, statErr := os.Stat(filepath.Join(pipelineDir, "requirement.md")); statErr != nil {
				continue
			}

			if !foundAny {
				fmt.Println("\n## 파일 기반 파이프라인")
				foundAny = true
			}

			// Detect stage from artifacts
			var existingFiles []string
			artifacts, _ := os.ReadDir(pipelineDir)
			for _, a := range artifacts {
				existingFiles = append(existingFiles, a.Name())
			}
			stage := domain.StageFromArtifacts(existingFiles)

			// Read status.json if exists
			status := "running"
			if data, readErr := os.ReadFile(filepath.Join(pipelineDir, "status.json")); readErr == nil {
				var sj map[string]string
				if json.Unmarshal(data, &sj) == nil {
					if s, ok := sj["status"]; ok {
						status = s
					}
				}
			}

			fmt.Printf("\nPipeline: %s\n", entry.Name())
			fmt.Printf("  Stage:  %s\n", stage)
			fmt.Printf("  Status: %s\n", status)

			sort.Strings(existingFiles)
			fmt.Println("  Artifacts:")
			for _, f := range existingFiles {
				fmt.Printf("    ✓ %s\n", f)
			}
		}
	}

	// Legacy: Show SQLite pipeline status
	dbPath := filepath.Join(root, ".pylon", "pylon.db")
	s, storeErr := store.NewStore(dbPath)
	if storeErr == nil {
		defer s.Close()
		s.Migrate()

		actives, listErr := s.GetActivePipelines()
		if listErr == nil && len(actives) > 0 {
			fmt.Println("\n## SQLite 파이프라인 (레거시)")
			foundAny = true
			for _, rec := range actives {
				pipeline, pErr := orchestrator.LoadPipeline([]byte(rec.StateJSON))
				if pErr != nil {
					fmt.Printf("Pipeline: %s (state parse error: %v)\n", rec.PipelineID, pErr)
					continue
				}
				fmt.Printf("\nPipeline: %s\n", pipeline.ID)
				fmt.Printf("  Stage:    %s\n", pipeline.CurrentStage)
				fmt.Printf("  Attempts: %d/%d\n", pipeline.Attempts, pipeline.MaxAttempts)
			}
		}
	}

	if !foundAny {
		fmt.Println("\nNo active pipeline.")
	}

	return nil
}
