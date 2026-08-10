package cli

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/kyago/pylon/internal/criteria"
)

func newInternalCriteriaCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "criteria", Hidden: true}
	cmd.AddCommand(newInternalCriteriaSnapshotCmd())
	return cmd
}

func newInternalCriteriaSnapshotCmd() *cobra.Command {
	var options criteria.CreateOptions
	cmd := &cobra.Command{
		Use:          "snapshot",
		Short:        "Create an immutable verification criteria snapshot",
		Hidden:       true,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			options.Now = time.Now
			snapshot, err := criteria.Create(options)
			if err != nil {
				return err
			}
			data, err := json.Marshal(snapshot)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}
	cmd.Flags().StringVar(&options.RunID, "run-id", "", "logical run ID")
	cmd.Flags().StringVar(&options.RepoID, "repo-id", "", "repository ID")
	cmd.Flags().StringVar(&options.BaseRevision, "base-revision", "", "repository base revision")
	cmd.Flags().StringVar(&options.WorkDir, "workdir", "", "project working directory")
	cmd.Flags().StringVar(&options.ConfigPath, "config", "", "live verify.yml path")
	cmd.Flags().StringVar(&options.AcceptancePath, "acceptance", "", "normalized acceptance criteria JSON path")
	cmd.Flags().StringVar(&options.OutputPath, "output", "", "criteria snapshot output path")
	cmd.Flags().StringVar(&options.ManifestPath, "manifest", "", "run manifest to bind to the snapshot")
	for _, name := range []string{"run-id", "repo-id", "base-revision", "workdir", "acceptance", "output", "manifest"} {
		_ = cmd.MarkFlagRequired(name)
	}
	return cmd
}
