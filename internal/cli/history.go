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
		Short: "Manage Fossil-backed workspace history",
	}
	cmd.AddCommand(
		newHistoryInitCmd(),
		newHistoryCheckpointCmd(),
		newHistoryLogCmd(),
		newHistoryShowCmd(),
		newHistoryDiffCmd(),
		newHistorySyncCmd(),
		newHistoryExportCmd(),
	)
	return cmd
}

func withHistoryManager(run func(*history.Manager) error) error {
	root, cfg, s, err := openWorkspaceStore()
	if err != nil {
		return err
	}
	defer s.Close()
	return run(history.NewManager(root, cfg.History, s, nil))
}

func newHistoryInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize the workspace Fossil history repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withHistoryManager(func(manager *history.Manager) error {
				if err := manager.Initialize(); err != nil {
					return err
				}
				if flagJSON {
					fmt.Println(`{"status":"ok","initialized":true}`)
				} else {
					fmt.Println("✓ Fossil 작업 이력 저장소가 준비되었습니다")
				}
				return nil
			})
		},
	}
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
				label := "체크인 완료"
				if result.Duplicate {
					label = "동일 내용 — 기존 체크인 유지"
				}
				fmt.Printf("✓ %s (%s)\n", label, result.Checkin)
				if result.PendingSync {
					fmt.Println("⚠ 원격 동기화 대기 중 — 'pylon history sync'로 재시도하세요")
				}
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
		Short: "Show Fossil check-in history",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withHistoryManager(func(manager *history.Manager) error {
				output, err := manager.Log(pipelineID, limit)
				if err == nil {
					if flagJSON {
						return printJSON(map[string]string{"output": output})
					}
					fmt.Println(output)
				}
				return err
			})
		},
	}
	cmd.Flags().StringVar(&pipelineID, "pipeline", "", "filter by pipeline ID")
	cmd.Flags().IntVar(&limit, "limit", 20, "maximum check-ins")
	return cmd
}

func newHistoryShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <checkin>",
		Short: "Show a Fossil check-in",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withHistoryManager(func(manager *history.Manager) error {
				output, err := manager.Show(args[0])
				if err == nil {
					if flagJSON {
						return printJSON(map[string]string{"checkin": args[0], "output": output})
					}
					fmt.Println(output)
				}
				return err
			})
		},
	}
}

func newHistoryDiffCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "diff <from> <to>",
		Short: "Diff two Fossil check-ins",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withHistoryManager(func(manager *history.Manager) error {
				output, err := manager.Diff(args[0], args[1])
				if err == nil {
					if flagJSON {
						return printJSON(map[string]string{"from": args[0], "to": args[1], "output": output})
					}
					fmt.Println(output)
				}
				return err
			})
		},
	}
}

func newHistorySyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Synchronize history with the configured Fossil remote",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withHistoryManager(func(manager *history.Manager) error {
				if err := manager.Sync(); err != nil {
					return err
				}
				if flagJSON {
					return printJSON(map[string]string{"status": "ok"})
				}
				fmt.Println("✓ Fossil 작업 이력 동기화 완료")
				return nil
			})
		},
	}
}

func newHistoryExportCmd() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "export <checkin>",
		Short: "Export a historical snapshot without changing runtime files",
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
