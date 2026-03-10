package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/orchestrator"
	"github.com/kyago/pylon/internal/store"
)

func newStageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stage",
		Short: "Manage pipeline stage transitions",
		Long: `파이프라인 상태를 관리합니다.

루트 에이전트(PO)가 파이프라인 진행 상태를 제어할 때 사용합니다.

사용 가능한 하위 명령:
  transition  상태 전이
  status      상태 조회
  list        파이프라인 목록`,
	}

	cmd.AddCommand(newStageTransitionCmd())
	cmd.AddCommand(newStageStatusCmd())
	cmd.AddCommand(newStageListCmd())

	return cmd
}

func newStageTransitionCmd() *cobra.Command {
	var pipelineID string
	var toStage string

	cmd := &cobra.Command{
		Use:   "transition",
		Short: "Transition pipeline to a new stage",
		RunE: func(cmd *cobra.Command, args []string) error {
			if pipelineID == "" || toStage == "" {
				return fmt.Errorf("--pipeline and --to are required")
			}
			return runStageTransition(pipelineID, toStage)
		},
	}

	cmd.Flags().StringVar(&pipelineID, "pipeline", "", "pipeline ID")
	cmd.Flags().StringVar(&toStage, "to", "", "target stage")

	return cmd
}

func newStageStatusCmd() *cobra.Command {
	var pipelineID string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show pipeline status",
		RunE: func(cmd *cobra.Command, args []string) error {
			if pipelineID == "" {
				return fmt.Errorf("--pipeline is required")
			}
			return runStageStatus(pipelineID)
		},
	}

	cmd.Flags().StringVar(&pipelineID, "pipeline", "", "pipeline ID")

	return cmd
}

func newStageListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all pipelines",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStageList()
		},
	}
}

func openWorkspaceStore() (string, *config.Config, *store.Store, error) {
	startDir := flagWorkspace
	if startDir == "" {
		startDir = "."
	}
	root, err := config.FindWorkspaceRoot(startDir)
	if err != nil {
		return "", nil, nil, fmt.Errorf("not in a pylon workspace")
	}

	cfg, err := config.LoadConfig(filepath.Join(root, ".pylon", "config.yml"))
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to load config: %w", err)
	}

	dbPath := filepath.Join(root, ".pylon", "pylon.db")
	s, err := store.NewStore(dbPath)
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to open store: %w", err)
	}

	if err := s.Migrate(); err != nil {
		s.Close()
		return "", nil, nil, fmt.Errorf("failed to migrate: %w", err)
	}

	return root, cfg, s, nil
}

func runStageTransition(pipelineID, toStage string) error {
	root, cfg, s, err := openWorkspaceStore()
	if err != nil {
		return err
	}
	defer s.Close()

	orch := orchestrator.NewOrchestrator(cfg, s, root)

	// Load existing pipeline
	rec, err := s.GetPipeline(pipelineID)
	if err != nil {
		return fmt.Errorf("failed to get pipeline: %w", err)
	}
	if rec == nil {
		return fmt.Errorf("pipeline %q not found", pipelineID)
	}

	pipeline, err := orchestrator.LoadPipeline([]byte(rec.StateJSON))
	if err != nil {
		return fmt.Errorf("failed to parse pipeline state: %w", err)
	}

	orch.Pipeline = pipeline

	target := orchestrator.Stage(toStage)
	if err := orch.TransitionTo(target); err != nil {
		return fmt.Errorf("transition failed: %w", err)
	}

	if flagJSON {
		data, _ := json.Marshal(map[string]string{
			"pipeline": pipelineID,
			"stage":    toStage,
			"status":   "ok",
		})
		fmt.Println(string(data))
	} else {
		fmt.Printf("✓ %s: %s → %s\n", pipelineID, rec.Stage, toStage)
	}

	return nil
}

func runStageStatus(pipelineID string) error {
	_, _, s, err := openWorkspaceStore()
	if err != nil {
		return err
	}
	defer s.Close()

	rec, err := s.GetPipeline(pipelineID)
	if err != nil {
		return fmt.Errorf("failed to get pipeline: %w", err)
	}
	if rec == nil {
		return fmt.Errorf("pipeline %q not found", pipelineID)
	}

	if flagJSON {
		fmt.Println(rec.StateJSON)
	} else {
		pipeline, err := orchestrator.LoadPipeline([]byte(rec.StateJSON))
		if err != nil {
			return fmt.Errorf("failed to parse pipeline: %w", err)
		}
		fmt.Printf("Pipeline: %s\n", pipeline.ID)
		fmt.Printf("Stage:    %s\n", pipeline.CurrentStage)
		fmt.Printf("Updated:  %s\n", rec.UpdatedAt.Format("2006-01-02 15:04:05"))
		if len(pipeline.Agents) > 0 {
			fmt.Println("Agents:")
			for name, agent := range pipeline.Agents {
				fmt.Printf("  - %s: %s (%s)\n", name, agent.Status, agent.AgentID)
			}
		}
		if len(pipeline.History) > 0 {
			fmt.Println("History:")
			for _, h := range pipeline.History {
				fmt.Printf("  %s → %s (%s)\n", h.From, h.To, h.CompletedAt.Format("15:04:05"))
			}
		}
	}

	return nil
}

func runStageList() error {
	_, _, s, err := openWorkspaceStore()
	if err != nil {
		return err
	}
	defer s.Close()

	records, err := s.ListAllPipelines()
	if err != nil {
		return fmt.Errorf("failed to list pipelines: %w", err)
	}

	if len(records) == 0 {
		if flagJSON {
			fmt.Println("[]")
		} else {
			fmt.Println("파이프라인이 없습니다")
		}
		return nil
	}

	if flagJSON {
		type pipelineOut struct {
			ID      string `json:"pipeline_id"`
			Stage   string `json:"stage"`
			Updated string `json:"updated_at"`
		}
		out := make([]pipelineOut, len(records))
		for i, r := range records {
			out[i] = pipelineOut{
				ID:      r.PipelineID,
				Stage:   r.Stage,
				Updated: r.UpdatedAt.Format("2006-01-02T15:04:05Z"),
			}
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Printf("%-30s %-20s %s\n", "PIPELINE", "STAGE", "UPDATED")
		for _, r := range records {
			fmt.Printf("%-30s %-20s %s\n", r.PipelineID, r.Stage, r.UpdatedAt.Format("2006-01-02 15:04:05"))
		}
	}

	return nil
}
