package cli

import (
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/kyago/pylon/internal/agent"
	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/executor"
	"github.com/kyago/pylon/internal/orchestrator"
)

func newResumeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resume [conversation-id]",
		Short: "Resume a Claude Code conversation by session ID",
		Long: `Resume a previously started Claude Code conversation using the session ID
stored in the conversation metadata.

This is useful for continuing PO conversations or agent sessions
that were interrupted.

Spec Reference: Section 9 "Conversation History"`,
		Args: cobra.ExactArgs(1),
		RunE: runResume,
	}
}

func runResume(cmd *cobra.Command, args []string) error {
	conversationID := args[0]

	// Find workspace
	startDir := flagWorkspace
	if startDir == "" {
		startDir = "."
	}
	root, err := config.FindWorkspaceRoot(startDir)
	if err != nil {
		return fmt.Errorf("not in a pylon workspace: %w", err)
	}

	// Load conversation
	convDir := filepath.Join(root, ".pylon", "conversations")
	convMgr := orchestrator.NewConversationManager(convDir)

	conv, err := convMgr.Load(conversationID)
	if err != nil {
		return fmt.Errorf("failed to load conversation %s: %w", conversationID, err)
	}

	if conv.Meta.Status != orchestrator.ConvStatusActive {
		return fmt.Errorf("conversation %s is not active (status: %s)", conversationID, conv.Meta.Status)
	}

	if conv.Meta.SessionID == "" {
		return fmt.Errorf("conversation %s has no session ID to resume", conversationID)
	}

	fmt.Printf("📋 Resuming conversation: %s\n", conversationID)
	fmt.Printf("🔗 Session: %s\n", conv.Meta.SessionID)
	fmt.Println()

	// Load config for environment setup
	cfg, err := config.LoadConfig(filepath.Join(root, ".pylon", "config.yml"))
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Determine work directory
	workDir := root
	projects, discoverErr := config.DiscoverProjects(root)
	if discoverErr != nil {
		fmt.Printf("⚠ Project discovery warning: %v\n", discoverErr)
	}
	if len(projects) > 0 {
		workDir = projects[0].Path
	}

	// Pre-validate claude CLI
	if _, lookErr := exec.LookPath("claude"); lookErr != nil {
		return fmt.Errorf("claude CLI not found: %w", lookErr)
	}

	// ExecInteractive with --resume flag
	directExec := executor.NewDirectExecutor()
	return directExec.ExecInteractive(executor.ExecConfig{
		Name:    "resume",
		Command: "claude",
		Args:    []string{"--resume", conv.Meta.SessionID},
		WorkDir: workDir,
		Env:     agent.ResolveEnv(cfg.Runtime.Env, nil),
	})
}
