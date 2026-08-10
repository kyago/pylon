package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kyago/pylon/internal/runstate"
)

func TestInternalStateCommandsCreateAndClaimTask(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".pylon", "config.yml"), []byte(`version: "0.2"`), 0644); err != nil {
		t.Fatal(err)
	}
	oldWorkspace := flagWorkspace
	flagWorkspace = root
	t.Cleanup(func() { flagWorkspace = oldWorkspace })

	executeStateCommand(t, "create-run", "run-1", "--requirement", "test")
	specPath := filepath.Join(root, "task.json")
	specData, err := json.Marshal(runstate.TaskSpec{RunID: "run-1", TaskID: "task-1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(specPath, specData, 0644); err != nil {
		t.Fatal(err)
	}
	executeStateCommand(t, "create-task", "--spec", specPath)
	executeStateCommand(t, "ready", "run-1", "task-1")

	output := executeStateCommand(t, "claim", "run-1", "task-1", "--owner", "worker", "--lease", "1m")
	var state runstate.TaskState
	if err := json.Unmarshal(output, &state); err != nil {
		t.Fatal(err)
	}
	if state.Status != runstate.TaskStatusClaimed || state.Attempt != 1 || state.Lease == nil || state.Lease.FencingToken == "" {
		t.Fatalf("claimed state = %+v", state)
	}

	shown := executeStateCommand(t, "show", "run-1", "task-1")
	var shownState runstate.TaskState
	if err := json.Unmarshal(shown, &shownState); err != nil {
		t.Fatal(err)
	}
	if shownState.Revision != state.Revision || shownState.Lease.FencingToken != state.Lease.FencingToken {
		t.Fatalf("shown state = %+v", shownState)
	}
}

func executeStateCommand(t *testing.T, args ...string) []byte {
	t.Helper()
	cmd := newInternalStateCmd()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("state %v failed: %v\n%s", args, err, output.String())
	}
	return output.Bytes()
}
