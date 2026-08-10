package runstate

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kyago/pylon/internal/provider"
)

func TestTaskLifecycleRequiresDependenciesAndVerification(t *testing.T) {
	store, _ := newTestStore(t)
	mustCreateRun(t, store, "run-1")
	mustCreateTask(t, store, TaskSpec{RunID: "run-1", TaskID: "task-a"})
	mustCreateTask(t, store, TaskSpec{RunID: "run-1", TaskID: "task-b", Dependencies: []string{"task-a"}})

	if _, err := store.MarkReady("run-1", "task-b"); !errors.Is(err, ErrDependencyPending) {
		t.Fatalf("dependent task became ready: %v", err)
	}
	if _, err := store.MarkReady("run-1", "task-a"); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.Claim("run-1", "task-a", "worker-a", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.Attempt != 1 || claimed.Lease == nil || claimed.Lease.Owner != "worker-a" {
		t.Fatalf("claimed state = %+v", claimed)
	}
	running, err := store.Start("run-1", "task-a", claimed.Lease.FencingToken, provider.WorkerHandle{
		Provider:   "fake",
		ExternalID: "worker-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if running.Status != TaskStatusRunning || running.Provider.Attempt != 1 {
		t.Fatalf("running state = %+v", running)
	}
	verifying, err := store.CompleteWork("run-1", "task-a", claimed.Lease.FencingToken, provider.TaskResult{
		State:        provider.WorkerStateSucceeded,
		ExitCode:     0,
		ChangedFiles: []string{"main.go"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if verifying.Status != TaskStatusVerifying {
		t.Fatalf("worker completion bypassed verification: %s", verifying.Status)
	}
	succeeded, err := store.FinalizeVerification("run-1", "task-a", Verification{
		DeterministicPassed: true,
		EvaluatorPassed:     true,
		EvidenceRefs:        []string{"verification.json"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if succeeded.Status != TaskStatusSucceeded {
		t.Fatalf("verified task status = %s", succeeded.Status)
	}
	if ready, err := store.MarkReady("run-1", "task-b"); err != nil || ready.Status != TaskStatusReady {
		t.Fatalf("dependent task did not become ready: %+v, %v", ready, err)
	}

	for _, path := range []string{
		filepath.Join(store.runDir("run-1"), "events.jsonl"),
		store.taskSpecPath("run-1", "task-a"),
		store.taskStatePath("run-1", "task-a"),
		filepath.Join(store.attemptDir("run-1", "task-a", 1), "lease.json"),
		filepath.Join(store.attemptDir("run-1", "task-a", 1), "provider.json"),
		filepath.Join(store.attemptDir("run-1", "task-a", 1), "result.json"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("missing durable artifact %s: %v", path, err)
		}
	}
}

func TestVerificationFailureProducesTerminalFailure(t *testing.T) {
	store, _ := newTestStore(t)
	mustCreateRun(t, store, "run-verify")
	mustCreateTask(t, store, TaskSpec{RunID: "run-verify", TaskID: "task"})
	claimed := mustStartTask(t, store, "run-verify", "task")
	if _, err := store.CompleteWork("run-verify", "task", claimed.Lease.FencingToken, provider.TaskResult{State: provider.WorkerStateSucceeded}); err != nil {
		t.Fatal(err)
	}
	failed, err := store.FinalizeVerification("run-verify", "task", Verification{
		DeterministicPassed: true,
		EvaluatorPassed:     false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != TaskStatusFailed {
		t.Fatalf("verification failure status = %s", failed.Status)
	}
}

func TestCreateRunAndTaskAreIdempotent(t *testing.T) {
	store, _ := newTestStore(t)
	runSpec := RunSpec{RunID: "run-create", Requirement: "test"}
	firstRun, err := store.CreateRun(runSpec)
	if err != nil {
		t.Fatal(err)
	}
	secondRun, err := store.CreateRun(runSpec)
	if err != nil {
		t.Fatal(err)
	}
	if firstRun != secondRun {
		t.Fatalf("idempotent run changed: first=%+v second=%+v", firstRun, secondRun)
	}

	taskSpec := TaskSpec{RunID: runSpec.RunID, TaskID: "task", Dependencies: []string{}}
	firstTask, err := store.CreateTask(taskSpec)
	if err != nil {
		t.Fatal(err)
	}
	eventsAfterCreate := mustReadEvents(t, store, runSpec.RunID)
	secondTask, err := store.CreateTask(taskSpec)
	if err != nil {
		t.Fatal(err)
	}
	if firstTask != secondTask {
		t.Fatalf("idempotent task changed: first=%+v second=%+v", firstTask, secondTask)
	}
	if got := len(mustReadEvents(t, store, runSpec.RunID)); got != len(eventsAfterCreate) {
		t.Fatalf("idempotent task creation appended an event: got %d want %d", got, len(eventsAfterCreate))
	}

	if _, err := store.CreateRun(RunSpec{RunID: runSpec.RunID, Requirement: "different"}); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("conflicting run creation error = %v", err)
	}
	if _, err := store.CreateTask(TaskSpec{RunID: runSpec.RunID, TaskID: "task", RepoID: "different"}); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("conflicting task creation error = %v", err)
	}
}

func TestExpiredLeaseSupportsResumeAndSemanticRetry(t *testing.T) {
	store, clock := newTestStore(t)
	mustCreateRun(t, store, "run-lease")
	mustCreateTask(t, store, TaskSpec{RunID: "run-lease", TaskID: "task"})
	claimed := mustStartTask(t, store, "run-lease", "task")
	firstToken := claimed.Lease.FencingToken

	*clock = clock.Add(30 * time.Second)
	heartbeat, err := store.Heartbeat("run-lease", "task", firstToken, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !heartbeat.Lease.ExpiresAt.Equal(clock.Add(time.Minute)) {
		t.Fatalf("heartbeat did not extend lease: %+v", heartbeat.Lease)
	}
	*clock = clock.Add(2 * time.Minute)
	interrupted, err := store.RecoverExpiredLeases("run-lease")
	if err != nil {
		t.Fatal(err)
	}
	if len(interrupted) != 1 || interrupted[0] != "task" {
		t.Fatalf("interrupted = %v", interrupted)
	}
	resumed, err := store.Resume("run-lease", "task", "worker-resumed", time.Minute, provider.WorkerHandle{
		Provider:    "fake",
		ExternalID:  "worker-1",
		ResumeToken: "resume-1",
		Attempt:     1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Attempt != 1 || resumed.Lease.FencingToken == firstToken {
		t.Fatalf("resume did not preserve attempt and rotate fence: %+v", resumed)
	}
	if _, err := store.Start("run-lease", "task", firstToken, *resumed.Provider); !errors.Is(err, ErrStaleFence) {
		t.Fatalf("old worker fence was accepted: %v", err)
	}
	if _, err := store.Start("run-lease", "task", resumed.Lease.FencingToken, *resumed.Provider); err != nil {
		t.Fatal(err)
	}

	*clock = clock.Add(2 * time.Minute)
	if _, err := store.RecoverExpiredLeases("run-lease"); err != nil {
		t.Fatal(err)
	}
	retried, err := store.Retry("run-lease", "task", "worker-new", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if retried.Attempt != 2 || retried.Provider != nil || retried.Lease.FencingToken == resumed.Lease.FencingToken {
		t.Fatalf("semantic retry state = %+v", retried)
	}
}

func TestDuplicateCompletionAndVerificationAreIdempotent(t *testing.T) {
	store, _ := newTestStore(t)
	mustCreateRun(t, store, "run-idempotent")
	mustCreateTask(t, store, TaskSpec{RunID: "run-idempotent", TaskID: "task"})
	claimed := mustStartTask(t, store, "run-idempotent", "task")
	result := provider.TaskResult{State: provider.WorkerStateSucceeded, ExitCode: 0}
	if _, err := store.CompleteWork("run-idempotent", "task", claimed.Lease.FencingToken, result); err != nil {
		t.Fatal(err)
	}
	eventsAfterCompletion := mustReadEvents(t, store, "run-idempotent")
	if _, err := store.CompleteWork("run-idempotent", "task", claimed.Lease.FencingToken, result); err != nil {
		t.Fatal(err)
	}
	if got := len(mustReadEvents(t, store, "run-idempotent")); got != len(eventsAfterCompletion) {
		t.Fatalf("duplicate completion appended event: %d -> %d", len(eventsAfterCompletion), got)
	}
	verification := Verification{DeterministicPassed: true, EvaluatorPassed: true, RecordedAt: store.now()}
	if _, err := store.FinalizeVerification("run-idempotent", "task", verification); err != nil {
		t.Fatal(err)
	}
	eventsAfterVerification := mustReadEvents(t, store, "run-idempotent")
	if _, err := store.FinalizeVerification("run-idempotent", "task", verification); err != nil {
		t.Fatal(err)
	}
	if got := len(mustReadEvents(t, store, "run-idempotent")); got != len(eventsAfterVerification) {
		t.Fatalf("duplicate verification appended event: %d -> %d", len(eventsAfterVerification), got)
	}
}

func TestEventReplayRepairsInterruptedMaterialization(t *testing.T) {
	store, _ := newTestStore(t)
	mustCreateRun(t, store, "run-recover")
	mustCreateTask(t, store, TaskSpec{RunID: "run-recover", TaskID: "task"})
	if _, err := store.MarkReady("run-recover", "task"); err != nil {
		t.Fatal(err)
	}

	originalWrite := store.writeJSON
	failStateWrite := true
	store.writeJSON = func(path string, value any) error {
		if failStateWrite && strings.HasSuffix(path, string(filepath.Separator)+"state.json") {
			failStateWrite = false
			return errors.New("simulated process crash after event append")
		}
		return originalWrite(path, value)
	}
	committed, err := store.Claim("run-recover", "task", "worker", time.Minute)
	if err == nil || committed.Status != TaskStatusClaimed {
		t.Fatalf("expected committed event with materialization failure, state=%+v err=%v", committed, err)
	}
	store.writeJSON = originalWrite

	restarted := NewStore(store.Root)
	restarted.Now = store.Now
	repaired, err := restarted.Recover("run-recover")
	if err != nil {
		t.Fatal(err)
	}
	if len(repaired) != 1 || repaired[0] != "task" {
		t.Fatalf("repaired = %v", repaired)
	}
	state, err := restarted.ReadTask("run-recover", "task")
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != TaskStatusClaimed || state.Revision != committed.Revision || state.Lease == nil {
		t.Fatalf("recovered state = %+v", state)
	}
}

func TestConcurrentClaimAllowsSingleOwner(t *testing.T) {
	store, _ := newTestStore(t)
	mustCreateRun(t, store, "run-concurrent")
	mustCreateTask(t, store, TaskSpec{RunID: "run-concurrent", TaskID: "task"})
	if _, err := store.MarkReady("run-concurrent", "task"); err != nil {
		t.Fatal(err)
	}

	var wait sync.WaitGroup
	wait.Add(2)
	results := make(chan error, 2)
	for _, owner := range []string{"worker-a", "worker-b"} {
		owner := owner
		go func() {
			defer wait.Done()
			_, err := store.Claim("run-concurrent", "task", owner, time.Minute)
			results <- err
		}()
	}
	wait.Wait()
	close(results)

	successes := 0
	invalidTransitions := 0
	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, ErrInvalidTransition) {
			invalidTransitions++
		} else {
			t.Fatalf("unexpected claim error: %v", err)
		}
	}
	if successes != 1 || invalidTransitions != 1 {
		t.Fatalf("claim results successes=%d invalid=%d", successes, invalidTransitions)
	}
	state, err := store.ReadTask("run-concurrent", "task")
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != TaskStatusClaimed || state.Attempt != 1 {
		t.Fatalf("concurrent state = %+v", state)
	}
}

func TestReplayDropsTornFinalEvent(t *testing.T) {
	store, _ := newTestStore(t)
	mustCreateRun(t, store, "run-torn")
	mustCreateTask(t, store, TaskSpec{RunID: "run-torn", TaskID: "task"})
	path := store.eventsPath("run-torn")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(`{"sequence":2`); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	events, err := store.ReadEvents("run-torn")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events after torn tail recovery = %d", len(events))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(data), "\n") || strings.Contains(string(data), `{"sequence":2`) {
		t.Fatalf("torn tail was not truncated: %q", data)
	}
}

func newTestStore(t *testing.T) (*Store, *time.Time) {
	t.Helper()
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	store := NewStore(t.TempDir())
	store.Now = func() time.Time { return now }
	tokenIndex := 0
	store.newToken = func() (string, error) {
		tokenIndex++
		return "token-" + time.Unix(int64(tokenIndex), 0).UTC().Format("150405"), nil
	}
	return store, &now
}

func mustCreateRun(t *testing.T, store *Store, runID string) {
	t.Helper()
	if _, err := store.CreateRun(RunSpec{RunID: runID, Requirement: "test"}); err != nil {
		t.Fatal(err)
	}
}

func mustCreateTask(t *testing.T, store *Store, spec TaskSpec) {
	t.Helper()
	if _, err := store.CreateTask(spec); err != nil {
		t.Fatal(err)
	}
}

func mustStartTask(t *testing.T, store *Store, runID, taskID string) TaskState {
	t.Helper()
	if _, err := store.MarkReady(runID, taskID); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.Claim(runID, taskID, "worker", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Start(runID, taskID, claimed.Lease.FencingToken, provider.WorkerHandle{Provider: "fake", ExternalID: "worker"}); err != nil {
		t.Fatal(err)
	}
	return claimed
}

func mustReadEvents(t *testing.T, store *Store, runID string) []Event {
	t.Helper()
	events, err := store.ReadEvents(runID)
	if err != nil {
		t.Fatal(err)
	}
	return events
}
