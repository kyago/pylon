package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newAddProjectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add-project [name]",
		Short: "Add a project as git submodule",
		Long: `Add a project to the workspace as a git submodule.
AI will analyze the codebase and suggest appropriate agent configurations.

Spec Reference: Section 7 "pylon add-project"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("not implemented yet (Phase 5)")
		},
	}

	cmd.Flags().String("repo", "", "git repository URL")

	return cmd
}
