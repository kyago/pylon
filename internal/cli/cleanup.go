package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/tmux"
)

func newCleanupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Clean up zombie tmux sessions",
		Long: `Identify and terminate zombie tmux sessions that were not properly
cleaned up after agent completion or abnormal termination.

Spec Reference: Section 7 "pylon cleanup"`,
		RunE: runCleanup,
	}

	cmd.Flags().BoolP("force", "f", false, "skip confirmation prompt")

	return cmd
}

func runCleanup(cmd *cobra.Command, args []string) error {
	force, _ := cmd.Flags().GetBool("force")

	// Find workspace
	startDir := flagWorkspace
	if startDir == "" {
		startDir = "."
	}
	root, err := config.FindWorkspaceRoot(startDir)
	if err != nil {
		return fmt.Errorf("not in a pylon workspace: %w", err)
	}

	// Load config for tmux prefix
	cfg, err := config.LoadConfig(root + "/.pylon/config.yml")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	mgr := tmux.NewManager(cfg.Tmux.SessionPrefix)
	sessions, err := mgr.List()
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}

	if len(sessions) == 0 {
		fmt.Println("No pylon sessions found. Nothing to clean up.")
		return nil
	}

	// Display sessions to be cleaned
	fmt.Printf("Found %d pylon session(s):\n", len(sessions))
	for _, s := range sessions {
		fmt.Printf("  ● %s\n", s.Name)
	}

	// Confirm unless --force
	if !force {
		fmt.Print("\nTerminate all listed sessions? (y/N): ")
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			fmt.Println("Cleanup cancelled.")
			return nil
		}
	}

	// Kill all sessions
	killed, err := mgr.KillAllWithPrefix()
	if err != nil {
		return fmt.Errorf("cleanup partially failed: %w (killed %d sessions)", err, killed)
	}

	fmt.Printf("Cleaned up %d session(s).\n", killed)
	return nil
}
