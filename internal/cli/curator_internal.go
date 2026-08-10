package cli

import (
	"fmt"
	"time"

	"github.com/kyago/pylon/internal/curator"
	"github.com/spf13/cobra"
)

func newInternalCuratorCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "curator", Hidden: true}
	cmd.AddCommand(newInternalCuratorProposeCmd())
	cmd.AddCommand(newInternalCuratorReviewCmd())
	cmd.AddCommand(newInternalCuratorGateCmd())
	return cmd
}

func newInternalCuratorProposeCmd() *cobra.Command {
	var checkpointRef, inputPath string
	cmd := &cobra.Command{
		Use:          "propose",
		Short:        "Create a learning candidate from a finalized checkpoint",
		Hidden:       true,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, _, err := openWorkspace()
			if err != nil {
				return err
			}
			var proposal curator.Proposal
			if err := decodeStrictJSON(inputPath, &proposal); err != nil {
				return err
			}
			candidate, err := curator.Create(curator.CreateOptions{
				Root: root, CheckpointRef: checkpointRef, Proposal: proposal, Now: time.Now,
			})
			if err != nil {
				return err
			}
			return writeCommandJSON(cmd, candidate)
		},
	}
	cmd.Flags().StringVar(&checkpointRef, "checkpoint", "", "finalized checkpoint ref <pipeline-id>/<completed|failed>")
	cmd.Flags().StringVar(&inputPath, "input", "", "structured candidate proposal JSON")
	_ = cmd.MarkFlagRequired("checkpoint")
	_ = cmd.MarkFlagRequired("input")
	return cmd
}

func newInternalCuratorReviewCmd() *cobra.Command {
	var candidateID, decision, reason string
	cmd := &cobra.Command{
		Use:          "review",
		Short:        "Approve or reject a curator candidate",
		Hidden:       true,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, _, err := openWorkspace()
			if err != nil {
				return err
			}
			status, err := curator.Review(root, candidateID, decision, reason, time.Now)
			if err != nil {
				return err
			}
			return writeCommandJSON(cmd, status)
		},
	}
	cmd.Flags().StringVar(&candidateID, "candidate", "", "candidate ID")
	cmd.Flags().StringVar(&decision, "decision", "", "approve or reject")
	cmd.Flags().StringVar(&reason, "reason", "", "human review reason")
	for _, name := range []string{"candidate", "decision", "reason"} {
		_ = cmd.MarkFlagRequired(name)
	}
	return cmd
}

func newInternalCuratorGateCmd() *cobra.Command {
	var candidateID, reportPath string
	cmd := &cobra.Command{
		Use:          "gate",
		Short:        "Record a passing acceptance corpus gate for an approved candidate",
		Hidden:       true,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, _, err := openWorkspace()
			if err != nil {
				return err
			}
			status, err := curator.Gate(root, candidateID, reportPath, time.Now)
			if err != nil {
				return err
			}
			if status.Status != "regression_passed" {
				return fmt.Errorf("candidate regression gate was not recorded")
			}
			return writeCommandJSON(cmd, status)
		},
	}
	cmd.Flags().StringVar(&candidateID, "candidate", "", "approved candidate ID")
	cmd.Flags().StringVar(&reportPath, "report", "", "passing acceptance corpus report")
	_ = cmd.MarkFlagRequired("candidate")
	_ = cmd.MarkFlagRequired("report")
	return cmd
}
