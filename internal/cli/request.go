package cli

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/git"
	"github.com/kyago/pylon/internal/orchestrator"
	"github.com/kyago/pylon/internal/slug"
	"github.com/kyago/pylon/internal/store"
	"github.com/kyago/pylon/internal/tmux"
)

func newRequestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "request [requirement]",
		Short: "Submit a new requirement for the AI agent team",
		Long: `Submit a natural language requirement for the AI agent team to implement.

The PO agent will analyze the requirement, ask clarifying questions,
and orchestrate the team to deliver the implementation.

Spec Reference: Section 7 "pylon request"`,
		Args: cobra.ExactArgs(1),
		RunE: runRequest,
	}
}

func runRequest(cmd *cobra.Command, args []string) error {
	requirement := args[0]

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
	cfg, err := config.LoadConfig(filepath.Join(root, ".pylon", "config.yml"))
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Open store
	dbPath := filepath.Join(root, ".pylon", "pylon.db")
	s, err := store.NewStore(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open store: %w", err)
	}
	defer s.Close()

	if err := s.Migrate(); err != nil {
		return fmt.Errorf("failed to migrate database: %w", err)
	}

	// Create tmux manager
	tmuxMgr := tmux.NewManager(cfg.Tmux.SessionPrefix)

	// Create orchestrator
	orch := orchestrator.NewOrchestrator(cfg, s, tmuxMgr, root)

	// Attempt recovery of previous state
	if err := orch.Recover(); err != nil {
		fmt.Printf("⚠ Recovery warning: %v\n", err)
	}

	// Generate pipeline ID
	pipelineID := generatePipelineID(requirement)

	fmt.Printf("📋 Pipeline: %s\n", pipelineID)
	fmt.Printf("📝 Requirement: %s\n", requirement)
	fmt.Println()

	// Step 1: Start pipeline
	if err := orch.StartPipeline(pipelineID); err != nil {
		return fmt.Errorf("failed to start pipeline: %w", err)
	}
	fmt.Println("✓ Pipeline created")

	// Step 2: Create conversation
	convDir := filepath.Join(root, ".pylon", "conversations")
	convMgr := orchestrator.NewConversationManager(convDir)

	conv, err := convMgr.Create(pipelineID, requirement)
	if err != nil {
		return fmt.Errorf("failed to create conversation: %w", err)
	}
	fmt.Printf("✓ Conversation: %s\n", conv.ID)

	// Step 3: Create git branch
	branch := git.BranchName(cfg.Git.BranchPrefix, slug.Slugify(requirement))
	fmt.Printf("✓ Branch: %s\n", branch)

	// Step 4: Transition to PO conversation
	if err := orch.TransitionTo(orchestrator.StagePOConversation); err != nil {
		return fmt.Errorf("failed to transition to PO conversation: %w", err)
	}

	fmt.Println()
	fmt.Println("🚀 Pipeline started. PO agent will begin shortly.")
	fmt.Println()
	fmt.Printf("  Monitor: pylon status\n")
	fmt.Printf("  Cancel:  pylon cancel %s\n", pipelineID)
	fmt.Printf("  Resume:  pylon resume %s\n", pipelineID)

	return nil
}

// generatePipelineID creates an ID in format "YYYYMMDD-slug".
func generatePipelineID(requirement string) string {
	date := time.Now().Format("20060102")
	s := slug.Slugify(requirement)
	if len(s) > 30 {
		s = s[:30]
	}
	s = strings.TrimRight(s, "-")
	return fmt.Sprintf("%s-%s", date, s)
}
