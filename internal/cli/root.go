// Package cli implements the Cobra-based CLI for pylon.
// Spec Reference: Section 7 "Command Interface"
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	// Global flags
	flagWorkspace string
	flagVerbose   bool
	flagJSON      bool

	// version is injected at build time via ldflags.
	version = "dev"
)

// SetVersion sets the version string (called from main).
func SetVersion(v string) {
	version = v
}

// rootCmd is the base command for pylon.
// When invoked without subcommands, it launches the Claude Code TUI session.
var rootCmd = &cobra.Command{
	Use:   "pylon",
	Short: "AI multi-agent development team orchestrator",
	Long: `Pylon - AI multi-agent development team orchestrator.

Pylon powers your AI agent team to build and ship software.

Users provide requirements, and the AI agent team handles
analysis, design, implementation, and PR creation.

Run 'pylon' without subcommands to launch the interactive AI session.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLaunch()
	},
}

// versionCmd prints build version information.
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("pylon version %s\n", version)
	},
}

func init() {
	// Global flags (Spec Section 7: --workspace, --verbose, --json)
	rootCmd.PersistentFlags().StringVar(&flagWorkspace, "workspace", "", "workspace path override")
	rootCmd.PersistentFlags().BoolVarP(&flagVerbose, "verbose", "v", false, "enable verbose output")
	rootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "output in JSON format")

	// Register subcommands
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(newDoctorCmd())
	rootCmd.AddCommand(newInitCmd())
	rootCmd.AddCommand(newRequestCmd())
	rootCmd.AddCommand(newStatusCmd())
	rootCmd.AddCommand(newCancelCmd())
	rootCmd.AddCommand(newReviewCmd())
	rootCmd.AddCommand(newDestroyCmd())
	rootCmd.AddCommand(newUninstallCmd())
	rootCmd.AddCommand(newAddProjectCmd())
	rootCmd.AddCommand(newDashboardCmd())
	rootCmd.AddCommand(newIndexCmd())
	rootCmd.AddCommand(newStageCmd())
	rootCmd.AddCommand(newMemCmd())
	rootCmd.AddCommand(newSyncMemoryCmd())
	rootCmd.AddCommand(newSyncProjectsCmd())
	rootCmd.AddCommand(newResumeCmd())
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
