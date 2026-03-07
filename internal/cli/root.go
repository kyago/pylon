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
var rootCmd = &cobra.Command{
	Use:   "pylon",
	Short: "AI multi-agent development team orchestrator",
	Long: `Pylon - AI multi-agent development team orchestrator.

Like the Protoss Pylon in StarCraft, Pylon is the energy source
that powers your AI agent team to build and ship software.

Users provide requirements, and the AI agent team handles
analysis, design, implementation, and PR creation.`,
	SilenceUsage:  true,
	SilenceErrors: true,
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
	rootCmd.AddCommand(newResumeCmd())
	rootCmd.AddCommand(newReviewCmd())
	rootCmd.AddCommand(newCleanupCmd())
	rootCmd.AddCommand(newDestroyCmd())
	rootCmd.AddCommand(newAddProjectCmd())
	rootCmd.AddCommand(newDashboardCmd())
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
