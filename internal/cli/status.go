package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/kyago/pylon/internal/domain"
	"github.com/kyago/pylon/internal/layout"
	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current work status",
		Long: `Display the status of running tasks and queued work items.

Shows file-based v2 pipelines (.pylon/runtime/).`,
		RunE: runStatus,
	}
}

func runStatus(cmd *cobra.Command, args []string) error {
	root, err := resolveRoot()
	if err != nil {
		return err
	}

	fmt.Println("Pylon Status")
	fmt.Println("─────────────────────────")

	foundAny := false

	// v2: Show file-based pipeline status
	runtimeDir := layout.RuntimeDir(root)
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

	if !foundAny {
		fmt.Println("\nNo active pipeline.")
	}

	return nil
}
