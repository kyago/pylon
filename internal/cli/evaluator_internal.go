package cli

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/kyago/pylon/internal/evaluator"
)

func newInternalEvaluatorCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "evaluator", Hidden: true}
	cmd.AddCommand(newInternalEvaluatorPrepareCmd())
	cmd.AddCommand(newInternalEvaluatorRecordCmd())
	return cmd
}

func newInternalEvaluatorRecordCmd() *cobra.Command {
	var options evaluator.RecordOptions
	cmd := &cobra.Command{
		Use:          "record",
		Short:        "Validate and record a structured evaluator verdict",
		Hidden:       true,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			options.Now = time.Now
			verdict, err := evaluator.Record(options)
			if err != nil {
				return err
			}
			data, err := json.Marshal(verdict)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}
	cmd.Flags().StringVar(&options.BundleDir, "bundle", "", "validated evaluator bundle directory")
	cmd.Flags().StringVar(&options.ManifestPath, "manifest", "", "repo run manifest bound to the evaluator request")
	cmd.Flags().StringVar(&options.InputPath, "input", "", "structured evaluator verdict JSON")
	cmd.Flags().StringVar(&options.OutputPath, "output", "", "validated evaluator result path")
	for _, name := range []string{"bundle", "manifest", "input", "output"} {
		_ = cmd.MarkFlagRequired(name)
	}
	return cmd
}

func newInternalEvaluatorPrepareCmd() *cobra.Command {
	var options evaluator.PrepareOptions
	cmd := &cobra.Command{
		Use:          "prepare",
		Short:        "Prepare an isolated read-only evaluator input bundle",
		Hidden:       true,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			options.Now = time.Now
			request, err := evaluator.Prepare(options)
			if err != nil {
				return err
			}
			data, err := json.Marshal(request)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}
	cmd.Flags().StringVar(&options.TaskID, "task-id", "", "task ID")
	cmd.Flags().StringVar(&options.ImplementationRoot, "implementation-root", "", "implementation worktree root")
	cmd.Flags().StringVar(&options.RequirementPath, "requirement", "", "original requirement file")
	cmd.Flags().StringVar(&options.CriteriaPath, "criteria", "", "criteria snapshot file")
	cmd.Flags().StringVar(&options.VerificationPath, "verification", "", "deterministic verification result")
	cmd.Flags().StringVar(&options.DiffPath, "diff", "", "implementation diff")
	cmd.Flags().StringVar(&options.TaskReportPath, "task-report", "", "optional structured task report")
	cmd.Flags().StringVar(&options.OutputDir, "output", "", "read-only evaluator bundle directory")
	cmd.Flags().StringVar(&options.ManifestPath, "manifest", "", "repo run manifest bound to the evaluator request")
	for _, name := range []string{"task-id", "implementation-root", "requirement", "criteria", "verification", "diff", "output", "manifest"} {
		_ = cmd.MarkFlagRequired(name)
	}
	return cmd
}
