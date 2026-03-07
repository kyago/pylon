package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/yongjunkang/pylon/internal/config"
	"github.com/yongjunkang/pylon/internal/orchestrator"
	"github.com/yongjunkang/pylon/internal/store"
	"github.com/yongjunkang/pylon/internal/tmux"
)

func newResumeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resume [pipeline-id]",
		Short: "Resume an interrupted pipeline",
		Long: `Resume a previously interrupted pipeline.
Recovers state from state.json, checks surviving tmux sessions,
and continues from the last known stage.

Spec Reference: Section 7 "pylon resume"`,
		Args: cobra.ExactArgs(1),
		RunE: runResume,
	}
}

func runResume(cmd *cobra.Command, args []string) error {
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

	tmuxMgr := tmux.NewManager(cfg.Tmux.SessionPrefix)
	orch := orchestrator.NewOrchestrator(cfg, s, tmuxMgr, root)

	// Recover state
	if err := orch.Recover(); err != nil {
		return fmt.Errorf("recovery failed: %w", err)
	}

	if orch.Pipeline == nil {
		return fmt.Errorf("no pipeline state to resume")
	}

	if orch.Pipeline.ID != pipelineID {
		return fmt.Errorf("active pipeline is %s, not %s", orch.Pipeline.ID, pipelineID)
	}

	if orch.Pipeline.IsTerminal() {
		return fmt.Errorf("pipeline %s is already in terminal state: %s", pipelineID, orch.Pipeline.CurrentStage)
	}

	fmt.Printf("🔄 Resuming pipeline: %s\n", pipelineID)
	fmt.Printf("  Current stage: %s\n", orch.Pipeline.CurrentStage)

	// Report agent status
	crashedCount := 0
	for name, agent := range orch.Pipeline.Agents {
		if agent.Status == "crashed" {
			fmt.Printf("  ⚠ Agent %s crashed (session: %s)\n", name, agent.TmuxSession)
			crashedCount++
		} else if agent.Status == "running" {
			fmt.Printf("  ● Agent %s still running\n", name)
		}
	}

	if crashedCount > 0 {
		fmt.Printf("\n  %d agent(s) need restart\n", crashedCount)
	}

	fmt.Println()
	fmt.Println("✓ Pipeline resumed. Continuing from current stage.")

	return nil
}
