package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kyago/pylon/internal/history"
	"github.com/kyago/pylon/internal/trajectory"
)

type fakeFailureCheckpointer struct {
	result history.CheckpointResult
	err    error
}

func (fake fakeFailureCheckpointer) Checkpoint(string, history.Phase) (history.CheckpointResult, error) {
	return fake.result, fake.err
}

func TestFinalizeFailureBindsManifestBeforeCheckpoint(t *testing.T) {
	root := t.TempDir()
	manifestPath := filepath.Join(root, "status.json")
	outputPath := filepath.Join(root, "failure-record.json")
	writeTrajectoryJSON(t, manifestPath, map[string]any{"pipeline_id": "run-1", "status": "running"})
	now := time.Date(2026, 8, 10, 4, 0, 0, 0, time.UTC)
	checkpoint := history.CheckpointResult{PipelineID: "run-1", Phase: history.PhaseFailed, Ref: "run-1/failed"}
	result, record, err := finalizeFailure("run-1", trajectory.FailureRecord{
		RunID: "run-1", TaskID: "T001", Attempt: 3, Phase: "verification", TerminalCause: "test_failure",
		EvidenceRefs: []string{"tasks/T001/attempts/003/stderr.log"}, AbandonedReason: "maximum attempts exhausted",
	}, outputPath, manifestPath, fakeFailureCheckpointer{result: checkpoint}, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if result.Ref != checkpoint.Ref || record.Digest == "" {
		t.Fatalf("unexpected finalization: result=%+v record=%+v", result, record)
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest["status"] != "failed" || manifest["failure_record"].(map[string]any)["digest"] != record.Digest {
		t.Fatalf("failure was not bound before checkpoint: %+v", manifest)
	}
}

func TestFinalizeFailurePreservesRuntimeWhenCheckpointFails(t *testing.T) {
	root := t.TempDir()
	manifestPath := filepath.Join(root, "status.json")
	outputPath := filepath.Join(root, "failure-record.json")
	writeTrajectoryJSON(t, manifestPath, map[string]any{"pipeline_id": "run-1", "status": "running"})
	checkpointErr := errors.New("history unavailable")
	_, _, err := finalizeFailure("run-1", trajectory.FailureRecord{
		RunID: "run-1", TaskID: "T001", Attempt: 1, Phase: "execution", TerminalCause: "worker_failed",
		EvidenceRefs: []string{"tasks/T001/stderr.log"}, AbandonedReason: "worker terminated",
	}, outputPath, manifestPath, fakeFailureCheckpointer{err: checkpointErr}, time.Now)
	if !errors.Is(err, checkpointErr) {
		t.Fatalf("checkpoint error = %v", err)
	}
	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("failure record was not preserved: %v", err)
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	cleanup := manifest["cleanup"].(map[string]any)
	if cleanup["status"] != "preserved" || cleanup["reason"] != "failed_checkpoint_failed" {
		t.Fatalf("runtime preservation missing: %+v", manifest)
	}
}

func TestInternalTrajectoryCommandsRegistered(t *testing.T) {
	internal := newInternalCmd()
	trajectoryCommand, _, err := internal.Find([]string{"trajectory"})
	if err != nil || trajectoryCommand == internal {
		t.Fatalf("trajectory command not registered: command=%v err=%v", trajectoryCommand, err)
	}
	for _, name := range []string{"task-report", "failure"} {
		command, _, err := trajectoryCommand.Find([]string{name})
		if err != nil || command == trajectoryCommand {
			t.Fatalf("trajectory %s command not registered: command=%v err=%v", name, command, err)
		}
	}
}

func writeTrajectoryJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}
