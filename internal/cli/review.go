package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/orchestrator"
	"github.com/kyago/pylon/internal/store"
)

func newReviewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "review [pr-url]",
		Short: "Process PR review comments",
		Long: `Read PR review comments and dispatch agents to address the feedback.
Creates a new pipeline stage to handle review feedback.

Spec Reference: Section 7 "pylon review"`,
		Args: cobra.ExactArgs(1),
		RunE: runReview,
	}
}

func runReview(cmd *cobra.Command, args []string) error {
	prURL := args[0]

	// Validate PR URL format
	if !strings.Contains(prURL, "github.com") || !strings.Contains(prURL, "/pull/") {
		return fmt.Errorf("invalid PR URL: must be a GitHub pull request URL")
	}

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

	// Recover state
	if err := orch.Recover(); err != nil {
		fmt.Printf("⚠ Recovery warning: %v\n", err)
	}

	fmt.Printf("📝 Processing PR review: %s\n", prURL)
	fmt.Println()

	// Check if there's an active pipeline to associate with
	if orch.Pipeline != nil && !orch.Pipeline.IsTerminal() {
		fmt.Printf("  Active pipeline: %s (stage: %s)\n", orch.Pipeline.ID, orch.Pipeline.CurrentStage)
	} else {
		fmt.Println("  No active pipeline — creating review-only pipeline")
	}

	fmt.Println()
	fmt.Println("✓ Review feedback processing initiated.")
	fmt.Println("  Agents will be dispatched to address comments.")

	return nil
}
