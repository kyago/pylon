package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/kyago/pylon/internal/fsutil"
	"github.com/kyago/pylon/internal/history"
	"github.com/kyago/pylon/internal/trajectory"
	"github.com/spf13/cobra"
)

type failureCheckpointer interface {
	Checkpoint(string, history.Phase) (history.CheckpointResult, error)
}

func newInternalTrajectoryCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "trajectory", Hidden: true}
	cmd.AddCommand(newInternalTaskReportCmd())
	cmd.AddCommand(newInternalFailureCmd())
	return cmd
}

func newInternalTaskReportCmd() *cobra.Command {
	var inputPath, outputPath string
	cmd := &cobra.Command{
		Use:          "task-report",
		Short:        "Validate and record a terminal task report",
		Hidden:       true,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, _, err := openWorkspace()
			if err != nil {
				return err
			}
			if err := requireRuntimeOutput(root, outputPath); err != nil {
				return err
			}
			var report trajectory.TaskReport
			if err := decodeStrictJSON(inputPath, &report); err != nil {
				return err
			}
			recorded, err := trajectory.RecordTaskReport(report, trajectory.RecordOptions{OutputPath: outputPath, Now: time.Now})
			if err != nil {
				return err
			}
			return writeCommandJSON(cmd, recorded)
		},
	}
	cmd.Flags().StringVar(&inputPath, "input", "", "structured task report JSON")
	cmd.Flags().StringVar(&outputPath, "output", "", "validated task report path under .pylon/runtime")
	_ = cmd.MarkFlagRequired("input")
	_ = cmd.MarkFlagRequired("output")
	return cmd
}

func newInternalFailureCmd() *cobra.Command {
	var pipelineID, inputPath, outputPath, manifestPath string
	cmd := &cobra.Command{
		Use:          "failure",
		Short:        "Record a terminal failure and create the failed checkpoint",
		Hidden:       true,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, _, err := openWorkspace()
			if err != nil {
				return err
			}
			if err := requireRuntimeOutput(root, outputPath); err != nil {
				return err
			}
			var record trajectory.FailureRecord
			if err := decodeStrictJSON(inputPath, &record); err != nil {
				return err
			}
			result, recorded, err := finalizeFailure(
				pipelineID,
				record,
				outputPath,
				manifestPath,
				history.NewManager(root),
				time.Now,
			)
			if err != nil {
				return err
			}
			return writeCommandJSON(cmd, map[string]any{"failure": recorded, "checkpoint": result})
		},
	}
	cmd.Flags().StringVar(&pipelineID, "pipeline", "", "root pipeline ID")
	cmd.Flags().StringVar(&inputPath, "input", "", "structured failure record JSON")
	cmd.Flags().StringVar(&outputPath, "output", "", "validated failure record path under .pylon/runtime")
	cmd.Flags().StringVar(&manifestPath, "manifest", "", "pipeline status manifest")
	for _, name := range []string{"pipeline", "input", "output", "manifest"} {
		_ = cmd.MarkFlagRequired(name)
	}
	return cmd
}

func finalizeFailure(
	pipelineID string,
	record trajectory.FailureRecord,
	outputPath string,
	manifestPath string,
	checkpointer failureCheckpointer,
	now func() time.Time,
) (history.CheckpointResult, trajectory.FailureRecord, error) {
	if record.RunID != pipelineID {
		return history.CheckpointResult{}, trajectory.FailureRecord{}, fmt.Errorf("failure run_id %q does not match pipeline %q", record.RunID, pipelineID)
	}
	if err := requireContained(filepath.Dir(manifestPath), outputPath); err != nil {
		return history.CheckpointResult{}, trajectory.FailureRecord{}, err
	}
	recorded, err := trajectory.RecordFailure(record, trajectory.RecordOptions{OutputPath: outputPath, Now: now})
	if err != nil {
		return history.CheckpointResult{}, trajectory.FailureRecord{}, err
	}
	if err := bindFailureManifest(manifestPath, outputPath, recorded); err != nil {
		return history.CheckpointResult{}, recorded, err
	}
	result, err := checkpointer.Checkpoint(pipelineID, history.PhaseFailed)
	if err != nil {
		preserveErr := markFailureCheckpointPreserved(manifestPath, err)
		if preserveErr != nil {
			return result, recorded, errors.Join(err, preserveErr)
		}
		return result, recorded, err
	}
	return result, recorded, nil
}

func bindFailureManifest(manifestPath, outputPath string, record trajectory.FailureRecord) error {
	return updateJSONManifest(manifestPath, func(manifest map[string]any) error {
		relative, err := filepath.Rel(filepath.Dir(manifestPath), outputPath)
		if err != nil {
			return err
		}
		manifest["stage"] = "failed"
		manifest["status"] = "failed"
		manifest["completed_at"] = record.RecordedAt.Format(time.RFC3339)
		manifest["failure_record"] = map[string]any{
			"path":        filepath.ToSlash(relative),
			"digest":      record.Digest,
			"recorded_at": record.RecordedAt.Format(time.RFC3339),
			"task_id":     record.TaskID,
			"attempt":     record.Attempt,
			"phase":       record.Phase,
		}
		return nil
	})
}

func markFailureCheckpointPreserved(manifestPath string, checkpointErr error) error {
	return updateJSONManifest(manifestPath, func(manifest map[string]any) error {
		manifest["cleanup"] = map[string]any{
			"status": "preserved",
			"reason": "failed_checkpoint_failed",
			"error":  checkpointErr.Error(),
		}
		return nil
	})
}

func updateJSONManifest(path string, mutate func(map[string]any) error) error {
	unlock, err := fsutil.AcquireFileLock(filepath.Join(filepath.Dir(path), ".trajectory-manifest.lock"), 10*time.Second)
	if err != nil {
		return err
	}
	defer unlock()
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("manifest JSON parse failed: %w", err)
	}
	if err := mutate(manifest); err != nil {
		return err
	}
	return fsutil.WriteJSONAtomic(path, manifest)
}

func decodeStrictJSON(path string, destination any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("%s JSON parse failed: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("%s contains multiple JSON values", path)
		}
		return fmt.Errorf("%s trailing JSON parse failed: %w", path, err)
	}
	return nil
}

func requireRuntimeOutput(root, outputPath string) error {
	return requireContained(filepath.Join(root, ".pylon", "runtime"), outputPath)
}

func requireContained(parent, child string) error {
	parentAbs, err := filepath.Abs(parent)
	if err != nil {
		return err
	}
	childAbs, err := filepath.Abs(child)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(parentAbs, childAbs)
	if err != nil {
		return err
	}
	if relative == "." || relative == ".." || filepath.IsAbs(relative) || len(relative) >= 3 && relative[:3] == ".."+string(filepath.Separator) {
		return fmt.Errorf("output must be inside %s: %s", parentAbs, childAbs)
	}
	return nil
}

func writeCommandJSON(cmd *cobra.Command, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
	return err
}
