package cli

import (
	"encoding/json"
	"fmt"

	"github.com/kyago/pylon/internal/history"
	"github.com/spf13/cobra"
)

func newHistoryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Manage file-based workspace history",
	}
	cmd.AddCommand(
		newHistoryCheckpointCmd(),
		newHistoryLogCmd(),
		newHistoryShowCmd(),
		newHistoryDiffCmd(),
		newHistoryExportCmd(),
	)
	return cmd
}

func withHistoryManager(run func(*history.Manager) error) error {
	root, _, err := openWorkspace()
	if err != nil {
		return err
	}
	return run(history.NewManager(root))
}

func newHistoryCheckpointCmd() *cobra.Command {
	var pipelineID, phase string
	cmd := &cobra.Command{
		Use:   "checkpoint",
		Short: "Create a curated pipeline history checkpoint",
		RunE: func(cmd *cobra.Command, args []string) error {
			if pipelineID == "" || phase == "" {
				return fmt.Errorf("--pipeline과 --phase가 필요합니다")
			}
			return withHistoryManager(func(manager *history.Manager) error {
				result, err := manager.Checkpoint(pipelineID, history.Phase(phase))
				if err != nil {
					return err
				}
				if flagJSON {
					return printJSON(result)
				}
				label := "체크포인트 기록 완료"
				if result.Duplicate {
					label = "동일 내용 — 기존 체크포인트 유지"
				}
				fmt.Printf("✓ %s (%s)\n", label, result.Ref)
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&pipelineID, "pipeline", "", "pipeline ID")
	cmd.Flags().StringVar(&phase, "phase", "", "planned, executed, completed, cancelled, or failed")
	return cmd
}

func newHistoryLogCmd() *cobra.Command {
	var pipelineID string
	var limit int
	cmd := &cobra.Command{
		Use:   "log",
		Short: "Show checkpoint history",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withHistoryManager(func(manager *history.Manager) error {
				entries, err := manager.Log(pipelineID, limit)
				if err != nil {
					return err
				}
				if flagJSON {
					return printJSON(entries)
				}
				if len(entries) == 0 {
					fmt.Println("기록된 체크포인트가 없습니다")
					return nil
				}
				for _, e := range entries {
					fmt.Printf("%s  %-30s %-10s %s\n",
						e.RecordedAt.Format("2006-01-02T15:04:05Z"), e.PipelineID, e.Phase, e.Status)
				}
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&pipelineID, "pipeline", "", "filter by pipeline ID")
	cmd.Flags().IntVar(&limit, "limit", 20, "maximum checkpoints")
	return cmd
}

func newHistoryShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <pipeline-id>/<phase>",
		Short: "Show a checkpoint snapshot",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withHistoryManager(func(manager *history.Manager) error {
				manifest, files, err := manager.Show(args[0])
				if err != nil {
					return err
				}
				if flagJSON {
					return printJSON(map[string]any{"manifest": manifest, "files": files})
				}
				fmt.Printf("ref:      %s/%s\n", manifest.PipelineID, manifest.Phase)
				fmt.Printf("recorded: %s\n", manifest.RecordedAt.Format("2006-01-02T15:04:05Z"))
				fmt.Printf("status:   %s\n", manifest.Status)
				fmt.Printf("projects: %v\n", manifest.AffectedProjects)
				fmt.Println("files:")
				for _, f := range files {
					fmt.Printf("  %s\n", f)
				}
				return nil
			})
		},
	}
}

func newHistoryDiffCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "diff <from-ref> <to-ref>",
		Short: "Diff two checkpoint snapshots",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withHistoryManager(func(manager *history.Manager) error {
				output, err := manager.Diff(args[0], args[1])
				if err != nil {
					return err
				}
				if flagJSON {
					return printJSON(map[string]string{"from": args[0], "to": args[1], "output": output})
				}
				fmt.Println(output)
				return nil
			})
		},
	}
}

func newHistoryExportCmd() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "export <pipeline-id>/<phase>",
		Short: "Export a checkpoint snapshot to a new directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if output == "" {
				return fmt.Errorf("--output이 필요합니다")
			}
			return withHistoryManager(func(manager *history.Manager) error {
				if err := manager.Export(args[0], output); err != nil {
					return err
				}
				if flagJSON {
					return printJSON(map[string]string{"status": "ok", "output": output})
				}
				fmt.Printf("✓ history snapshot export 완료: %s\n", output)
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&output, "output", "", "new destination directory")
	return cmd
}

func printJSON(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func printJSONIndent(value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
