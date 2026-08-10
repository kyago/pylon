package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/kyago/pylon/internal/provider"
	"github.com/kyago/pylon/internal/runstate"
	"github.com/spf13/cobra"
)

func newInternalStateCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "state", Hidden: true}
	cmd.AddCommand(
		newStateCreateRunCmd(),
		newStateCreateTaskCmd(),
		newStateReadyCmd(),
		newStateClaimCmd(),
		newStateStartCmd(),
		newStateHeartbeatCmd(),
		newStateCompleteCmd(),
		newStateVerifyCmd(),
		newStateResumeCmd(),
		newStateRetryCmd(),
		newStateCancelCmd(),
		newStateRecoverCmd(),
		newStateShowCmd(),
	)
	return cmd
}

func newStateCreateRunCmd() *cobra.Command {
	var requirement string
	cmd := &cobra.Command{
		Use:    "create-run <run-id>",
		Args:   cobra.ExactArgs(1),
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := workspaceStateStore()
			if err != nil {
				return err
			}
			state, err := store.CreateRun(runstate.RunSpec{RunID: args[0], Requirement: requirement})
			return writeStateOutput(cmd, state, err)
		},
	}
	cmd.Flags().StringVar(&requirement, "requirement", "", "original run requirement")
	return cmd
}

func newStateCreateTaskCmd() *cobra.Command {
	var specPath string
	cmd := &cobra.Command{
		Use:    "create-task",
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var spec runstate.TaskSpec
			if err := readStateJSON(specPath, &spec); err != nil {
				return err
			}
			store, err := workspaceStateStore()
			if err != nil {
				return err
			}
			state, err := store.CreateTask(spec)
			return writeStateOutput(cmd, state, err)
		},
	}
	cmd.Flags().StringVar(&specPath, "spec", "", "task spec JSON path")
	_ = cmd.MarkFlagRequired("spec")
	return cmd
}

func newStateReadyCmd() *cobra.Command {
	return newSimpleTaskStateCmd("ready", func(store *runstate.Store, runID, taskID string) (runstate.TaskState, error) {
		return store.MarkReady(runID, taskID)
	})
}

func newStateClaimCmd() *cobra.Command {
	var owner, leaseText string
	cmd := newTaskArgsCmd("claim", func(cmd *cobra.Command, runID, taskID string) error {
		lease, err := time.ParseDuration(leaseText)
		if err != nil {
			return fmt.Errorf("invalid lease duration: %w", err)
		}
		store, err := workspaceStateStore()
		if err != nil {
			return err
		}
		state, err := store.Claim(runID, taskID, owner, lease)
		return writeStateOutput(cmd, state, err)
	})
	cmd.Flags().StringVar(&owner, "owner", "", "worker owner id")
	cmd.Flags().StringVar(&leaseText, "lease", "1m", "lease duration")
	_ = cmd.MarkFlagRequired("owner")
	return cmd
}

func newStateStartCmd() *cobra.Command {
	var fence, handlePath string
	cmd := newTaskArgsCmd("start", func(cmd *cobra.Command, runID, taskID string) error {
		var handle provider.WorkerHandle
		if err := readStateJSON(handlePath, &handle); err != nil {
			return err
		}
		store, err := workspaceStateStore()
		if err != nil {
			return err
		}
		state, err := store.Start(runID, taskID, fence, handle)
		return writeStateOutput(cmd, state, err)
	})
	cmd.Flags().StringVar(&fence, "fence", "", "attempt fencing token")
	cmd.Flags().StringVar(&handlePath, "handle", "", "provider handle JSON path")
	_ = cmd.MarkFlagRequired("fence")
	_ = cmd.MarkFlagRequired("handle")
	return cmd
}

func newStateHeartbeatCmd() *cobra.Command {
	var fence, leaseText string
	cmd := newTaskArgsCmd("heartbeat", func(cmd *cobra.Command, runID, taskID string) error {
		lease, err := time.ParseDuration(leaseText)
		if err != nil {
			return fmt.Errorf("invalid lease duration: %w", err)
		}
		store, err := workspaceStateStore()
		if err != nil {
			return err
		}
		state, err := store.Heartbeat(runID, taskID, fence, lease)
		return writeStateOutput(cmd, state, err)
	})
	cmd.Flags().StringVar(&fence, "fence", "", "attempt fencing token")
	cmd.Flags().StringVar(&leaseText, "lease", "1m", "lease duration")
	_ = cmd.MarkFlagRequired("fence")
	return cmd
}

func newStateCompleteCmd() *cobra.Command {
	var fence, resultPath string
	cmd := newTaskArgsCmd("complete", func(cmd *cobra.Command, runID, taskID string) error {
		var result provider.TaskResult
		if err := readStateJSON(resultPath, &result); err != nil {
			return err
		}
		store, err := workspaceStateStore()
		if err != nil {
			return err
		}
		state, err := store.CompleteWork(runID, taskID, fence, result)
		return writeStateOutput(cmd, state, err)
	})
	cmd.Flags().StringVar(&fence, "fence", "", "attempt fencing token")
	cmd.Flags().StringVar(&resultPath, "result", "", "task result JSON path")
	_ = cmd.MarkFlagRequired("fence")
	_ = cmd.MarkFlagRequired("result")
	return cmd
}

func newStateVerifyCmd() *cobra.Command {
	var deterministic, evaluator bool
	var evidence []string
	cmd := newTaskArgsCmd("verify", func(cmd *cobra.Command, runID, taskID string) error {
		store, err := workspaceStateStore()
		if err != nil {
			return err
		}
		state, err := store.FinalizeVerification(runID, taskID, runstate.Verification{
			DeterministicPassed: deterministic,
			EvaluatorPassed:     evaluator,
			EvidenceRefs:        evidence,
		})
		return writeStateOutput(cmd, state, err)
	})
	cmd.Flags().BoolVar(&deterministic, "deterministic", false, "deterministic verification passed")
	cmd.Flags().BoolVar(&evaluator, "evaluator", false, "isolated evaluator passed")
	cmd.Flags().StringSliceVar(&evidence, "evidence", nil, "verification evidence references")
	return cmd
}

func newStateResumeCmd() *cobra.Command {
	var owner, leaseText, handlePath string
	cmd := newTaskArgsCmd("resume", func(cmd *cobra.Command, runID, taskID string) error {
		lease, err := time.ParseDuration(leaseText)
		if err != nil {
			return fmt.Errorf("invalid lease duration: %w", err)
		}
		var handle provider.WorkerHandle
		if err := readStateJSON(handlePath, &handle); err != nil {
			return err
		}
		store, err := workspaceStateStore()
		if err != nil {
			return err
		}
		state, err := store.Resume(runID, taskID, owner, lease, handle)
		return writeStateOutput(cmd, state, err)
	})
	cmd.Flags().StringVar(&owner, "owner", "", "worker owner id")
	cmd.Flags().StringVar(&leaseText, "lease", "1m", "lease duration")
	cmd.Flags().StringVar(&handlePath, "handle", "", "resumed provider handle JSON path")
	_ = cmd.MarkFlagRequired("owner")
	_ = cmd.MarkFlagRequired("handle")
	return cmd
}

func newStateRetryCmd() *cobra.Command {
	var owner, leaseText string
	cmd := newTaskArgsCmd("retry", func(cmd *cobra.Command, runID, taskID string) error {
		lease, err := time.ParseDuration(leaseText)
		if err != nil {
			return fmt.Errorf("invalid lease duration: %w", err)
		}
		store, err := workspaceStateStore()
		if err != nil {
			return err
		}
		state, err := store.Retry(runID, taskID, owner, lease)
		return writeStateOutput(cmd, state, err)
	})
	cmd.Flags().StringVar(&owner, "owner", "", "worker owner id")
	cmd.Flags().StringVar(&leaseText, "lease", "1m", "lease duration")
	_ = cmd.MarkFlagRequired("owner")
	return cmd
}

func newStateCancelCmd() *cobra.Command {
	var message string
	cmd := newTaskArgsCmd("cancel", func(cmd *cobra.Command, runID, taskID string) error {
		store, err := workspaceStateStore()
		if err != nil {
			return err
		}
		state, err := store.Cancel(runID, taskID, message)
		return writeStateOutput(cmd, state, err)
	})
	cmd.Flags().StringVar(&message, "message", "cancelled by control plane", "cancellation reason")
	return cmd
}

func newStateRecoverCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "recover <run-id>",
		Args:   cobra.ExactArgs(1),
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := workspaceStateStore()
			if err != nil {
				return err
			}
			repaired, err := store.Recover(args[0])
			if err != nil {
				return err
			}
			interrupted, err := store.RecoverExpiredLeases(args[0])
			return writeStateOutput(cmd, map[string]any{"repaired": repaired, "interrupted": interrupted}, err)
		},
	}
}

func newStateShowCmd() *cobra.Command {
	return newSimpleTaskStateCmd("show", func(store *runstate.Store, runID, taskID string) (runstate.TaskState, error) {
		return store.ReadTask(runID, taskID)
	})
}

func newSimpleTaskStateCmd(name string, action func(*runstate.Store, string, string) (runstate.TaskState, error)) *cobra.Command {
	return newTaskArgsCmd(name, func(cmd *cobra.Command, runID, taskID string) error {
		store, err := workspaceStateStore()
		if err != nil {
			return err
		}
		state, err := action(store, runID, taskID)
		return writeStateOutput(cmd, state, err)
	})
}

func newTaskArgsCmd(name string, action func(*cobra.Command, string, string) error) *cobra.Command {
	return &cobra.Command{
		Use:    name + " <run-id> <task-id>",
		Args:   cobra.ExactArgs(2),
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return action(cmd, args[0], args[1])
		},
	}
}

func workspaceStateStore() (*runstate.Store, error) {
	root, err := resolveRoot()
	if err != nil {
		return nil, err
	}
	return runstate.NewStore(root), nil
}

func readStateJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, value); err != nil {
		return fmt.Errorf("failed to parse %s: %w", path, err)
	}
	return nil
}

func writeStateOutput(cmd *cobra.Command, value any, err error) error {
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
