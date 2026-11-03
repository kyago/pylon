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
	Short: "Workspace tool for Claude Code and Codex across multiple repositories",
	Long: `Pylon - workspace tool for Claude Code and Codex across multiple repositories.

Pylon sets up a workspace root over several repositories: the root AGENTS.md,
project list, agents, and a simple project memory. Start sessions yourself in
Claude Code, Codex or Paseo.`,
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
	rootCmd.AddCommand(newUninstallCmd())
	rootCmd.AddCommand(newAddProjectCmd())
	rootCmd.AddCommand(newDeleteProjectCmd())
	rootCmd.AddCommand(newMemCmd())
	rootCmd.AddCommand(newUpdateCmd())
	rootCmd.AddCommand(newInternalCmd())
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
