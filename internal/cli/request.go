package cli

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/kyago/pylon/internal/agent"
	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/executor"
	"github.com/kyago/pylon/internal/git"
	"github.com/kyago/pylon/internal/memory"
	"github.com/kyago/pylon/internal/orchestrator"
	"github.com/kyago/pylon/internal/slug"
	"github.com/kyago/pylon/internal/store"
)

var flagContinue string

func newRequestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "request [requirement]",
		Short: "Submit a new requirement for the AI agent team",
		Long: `Submit a natural language requirement for the AI agent team to implement.

The PO agent will analyze the requirement, ask clarifying questions,
and orchestrate the team to deliver the implementation.

Use --continue to resume a pipeline after PO conversation completes.

Spec Reference: Section 7 "pylon request"`,
		Args: cobra.ExactArgs(1),
		RunE: runRequest,
	}

	cmd.Flags().StringVar(&flagContinue, "continue", "", "continue a pipeline from the given ID (after PO conversation)")

	return cmd
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

	// Load agents and projects
	agents, err := config.LoadAllAgents(root)
	if err != nil {
		fmt.Printf("⚠ Agent loading warning: %v\n", err)
		agents = make(map[string]*config.AgentConfig)
	}

	projects, err := config.DiscoverProjects(root)
	if err != nil {
		fmt.Printf("⚠ Project discovery warning: %v\n", err)
	}

	if len(projects) == 0 {
		return fmt.Errorf("no projects found in workspace. Run 'pylon add-project' first")
	}

	// Generate or use provided pipeline ID
	pipelineID := flagContinue
	if pipelineID == "" {
		pipelineID = generatePipelineID(requirement)
	}

	// Create git branch
	branch := git.BranchName(cfg.Git.BranchPrefix, slug.Slugify(requirement))

	// Create conversation (if new pipeline)
	convDir := filepath.Join(root, ".pylon", "conversations")
	convMgr := orchestrator.NewConversationManager(convDir)

	if flagContinue == "" {
		fmt.Printf("📋 Pipeline: %s\n", pipelineID)
		fmt.Printf("📝 Requirement: %s\n", requirement)
		fmt.Printf("🌿 Branch: %s\n", branch)
		fmt.Println()

		if _, err := convMgr.Create(pipelineID, requirement); err != nil {
			return fmt.Errorf("failed to create conversation: %w", err)
		}
	} else {
		fmt.Printf("📋 Resuming pipeline: %s\n", pipelineID)
		fmt.Println()
	}

	// Build dependencies
	memMgr := memory.NewManager(s, cfg.Memory)
	runner := agent.NewRunner(executor.NewDirectExecutor())
	wtMgr := git.NewWorktreeManager(cfg.Git.Worktree.Enabled, cfg.Git.Worktree.AutoCleanup)

	// Create and run the orchestration loop
	loop := orchestrator.NewLoop(orchestrator.LoopConfig{
		Config:      cfg,
		Store:       s,
		WorkDir:     root,
		PipelineID:  pipelineID,
		Requirement: requirement,
		Branch:      branch,
		MemManager:  memMgr,
		Runner:      runner,
		WorktreeMgr: wtMgr,
		Agents:      agents,
		Projects:    projects,
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	err = loop.Run(ctx)

	if errors.Is(err, orchestrator.ErrInteractiveRequired) {
		// PO conversation stage reached — launch PO interactively
		fmt.Println("🚀 PO 에이전트와 대화를 시작합니다...")
		fmt.Println()
		fmt.Printf("  대화 완료 후 headless 실행: pylon request --continue %s \"%s\"\n", pipelineID, requirement)
		fmt.Println()

		// Launch PO agent interactively
		poAgent := agents["po"]
		if poAgent == nil {
			return fmt.Errorf("PO agent not found in workspace agents")
		}
		poAgent.ResolveDefaults(cfg)

		poRunner := agent.NewRunner(executor.NewDirectExecutor())
		claudeMD, _ := (&agent.ClaudeMDBuilder{MaxLines: 200}).Build(agent.BuildInput{
			CommunicationRules: agent.DefaultCommunicationRules(),
			TaskContext:        fmt.Sprintf("요구사항 분석: %s", requirement),
			CompactionRules:    agent.DefaultCompactionRules(),
		})

		// Pre-validate claude CLI before state transition
		if _, lookErr := exec.LookPath("claude"); lookErr != nil {
			return fmt.Errorf("claude CLI not found: %w", lookErr)
		}

		// Transition past PO for the --continue path
		// (ExecInteractive replaces the process on success, so state must be saved before)
		orch := loop.GetOrchestrator()
		if err := orch.TransitionTo(orchestrator.StageArchitectAnalysis); err != nil {
			return fmt.Errorf("failed to transition to architect analysis: %w", err)
		}

		execErr := poRunner.Executor.ExecInteractive(executor.ExecConfig{
			Name:    "po",
			Command: "claude",
			Args: poRunner.BuildArgs(agent.RunConfig{
				Agent:       poAgent,
				Global:      cfg,
				Interactive: true,
				ClaudeMD:    claudeMD,
			}),
			WorkDir: projects[0].Path,
			Env:     agent.ResolveEnv(cfg.Runtime.Env, poAgent.Env),
		})

		// ExecInteractive returns only on failure — rollback state
		_ = orch.ForceStage(orchestrator.StagePOConversation)
		return fmt.Errorf("failed to launch PO agent: %w", execErr)
	}

	if err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("✅ Pipeline completed successfully!")
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
