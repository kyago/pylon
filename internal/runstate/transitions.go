package runstate

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/kyago/pylon/internal/provider"
)

var errNoChange = errors.New("task state unchanged")

type mutation func(*TaskState, map[string]TaskState) error

func (store *Store) MarkReady(runID, taskID string) (TaskState, error) {
	return store.mutate(runID, taskID, EventTaskReady, func(state *TaskState, states map[string]TaskState) error {
		if state.Status == TaskStatusReady {
			return errNoChange
		}
		if state.Status != TaskStatusPending && state.Status != TaskStatusBlocked {
			return transitionError(state.Status, TaskStatusReady)
		}
		spec, err := readJSON[TaskSpec](store.taskSpecPath(runID, taskID))
		if err != nil {
			return err
		}
		for _, dependency := range spec.Dependencies {
			dependencyState, ok := states[dependency]
			if !ok || dependencyState.Status != TaskStatusSucceeded {
				return fmt.Errorf("%w: %s", ErrDependencyPending, dependency)
			}
		}
		state.Status = TaskStatusReady
		state.Message = ""
		return nil
	})
}

func (store *Store) Block(runID, taskID, message string) (TaskState, error) {
	return store.mutate(runID, taskID, EventTaskBlocked, func(state *TaskState, _ map[string]TaskState) error {
		if state.Status == TaskStatusBlocked && state.Message == message {
			return errNoChange
		}
		if state.Status != TaskStatusPending && state.Status != TaskStatusReady {
			return transitionError(state.Status, TaskStatusBlocked)
		}
		state.Status = TaskStatusBlocked
		state.Message = message
		return nil
	})
}

func (store *Store) Claim(runID, taskID, owner string, duration time.Duration) (TaskState, error) {
	return store.claim(runID, taskID, owner, duration, false, nil, EventTaskClaimed)
}

func (store *Store) Retry(runID, taskID, owner string, duration time.Duration) (TaskState, error) {
	return store.claim(runID, taskID, owner, duration, true, nil, EventTaskRetried)
}

func (store *Store) Resume(runID, taskID, owner string, duration time.Duration, handle provider.WorkerHandle) (TaskState, error) {
	return store.claim(runID, taskID, owner, duration, false, &handle, EventTaskResumed)
}

func (store *Store) claim(runID, taskID, owner string, duration time.Duration, retry bool, resumedHandle *provider.WorkerHandle, eventType EventType) (TaskState, error) {
	if strings.TrimSpace(owner) == "" {
		return TaskState{}, fmt.Errorf("claim owner is required")
	}
	if duration <= 0 {
		return TaskState{}, fmt.Errorf("lease duration must be positive")
	}
	return store.mutate(runID, taskID, eventType, func(state *TaskState, _ map[string]TaskState) error {
		switch eventType {
		case EventTaskClaimed:
			if state.Status != TaskStatusReady {
				return transitionError(state.Status, TaskStatusClaimed)
			}
			state.Attempt++
		case EventTaskRetried:
			if state.Status != TaskStatusInterrupted || !retry {
				return transitionError(state.Status, TaskStatusClaimed)
			}
			state.Attempt++
			state.Provider = nil
			state.Result = nil
			state.Verification = nil
		case EventTaskResumed:
			if state.Status != TaskStatusInterrupted || resumedHandle == nil {
				return transitionError(state.Status, TaskStatusClaimed)
			}
			if state.Attempt == 0 {
				return fmt.Errorf("interrupted task has no attempt")
			}
			if resumedHandle.Attempt != 0 && resumedHandle.Attempt != state.Attempt {
				return fmt.Errorf("resume handle attempt %d does not match task attempt %d", resumedHandle.Attempt, state.Attempt)
			}
			handle := *resumedHandle
			handle.Attempt = state.Attempt
			state.Provider = &handle
		}
		token, err := store.token()
		if err != nil {
			return err
		}
		now := store.now()
		state.Status = TaskStatusClaimed
		state.Lease = &Lease{
			Owner:        owner,
			Attempt:      state.Attempt,
			FencingToken: token,
			HeartbeatAt:  now,
			ExpiresAt:    now.Add(duration),
		}
		state.Message = ""
		return nil
	})
}

func (store *Store) Start(runID, taskID, fencingToken string, handle provider.WorkerHandle) (TaskState, error) {
	return store.mutate(runID, taskID, EventTaskStarted, func(state *TaskState, _ map[string]TaskState) error {
		if state.Status != TaskStatusClaimed {
			return transitionError(state.Status, TaskStatusRunning)
		}
		if err := store.validateLease(*state, fencingToken); err != nil {
			return err
		}
		if handle.Provider == "" {
			return fmt.Errorf("provider handle name is required")
		}
		if handle.Attempt != 0 && handle.Attempt != state.Attempt {
			return fmt.Errorf("provider handle attempt %d does not match task attempt %d", handle.Attempt, state.Attempt)
		}
		handle.Attempt = state.Attempt
		state.Provider = &handle
		state.Status = TaskStatusRunning
		return nil
	})
}

func (store *Store) Heartbeat(runID, taskID, fencingToken string, duration time.Duration) (TaskState, error) {
	if duration <= 0 {
		return TaskState{}, fmt.Errorf("lease duration must be positive")
	}
	return store.mutate(runID, taskID, EventTaskHeartbeat, func(state *TaskState, _ map[string]TaskState) error {
		if state.Status != TaskStatusClaimed && state.Status != TaskStatusRunning {
			return transitionError(state.Status, state.Status)
		}
		if err := store.validateLease(*state, fencingToken); err != nil {
			return err
		}
		now := store.now()
		state.Lease.HeartbeatAt = now
		state.Lease.ExpiresAt = now.Add(duration)
		return nil
	})
}

func (store *Store) CompleteWork(runID, taskID, fencingToken string, result provider.TaskResult) (TaskState, error) {
	return store.mutate(runID, taskID, EventTaskWorkReported, func(state *TaskState, _ map[string]TaskState) error {
		if state.Status != TaskStatusRunning {
			if state.Result != nil && reflect.DeepEqual(*state.Result, result) {
				return errNoChange
			}
			return transitionError(state.Status, TaskStatusVerifying)
		}
		if err := store.validateLease(*state, fencingToken); err != nil {
			return err
		}
		resultCopy := cloneResult(result)
		state.Result = &resultCopy
		switch result.State {
		case provider.WorkerStateSucceeded:
			state.Status = TaskStatusVerifying
		case provider.WorkerStateFailed:
			state.Status = TaskStatusFailed
		case provider.WorkerStateInterrupted:
			state.Status = TaskStatusInterrupted
		case provider.WorkerStateCancelled:
			state.Status = TaskStatusCancelled
		default:
			return fmt.Errorf("worker result state %q is not terminal", result.State)
		}
		return nil
	})
}

func (store *Store) FinalizeVerification(runID, taskID string, verification Verification) (TaskState, error) {
	return store.mutate(runID, taskID, EventTaskVerified, func(state *TaskState, _ map[string]TaskState) error {
		if state.Status != TaskStatusVerifying {
			if verification.RecordedAt.IsZero() && state.Verification != nil {
				verification.RecordedAt = state.Verification.RecordedAt
			}
			verification.RecordedAt = verification.RecordedAt.UTC()
			if state.Verification != nil && reflect.DeepEqual(*state.Verification, verification) {
				return errNoChange
			}
			return transitionError(state.Status, TaskStatusSucceeded)
		}
		verification.RecordedAt = verification.RecordedAt.UTC()
		if verification.RecordedAt.IsZero() {
			verification.RecordedAt = store.now()
		}
		verificationCopy := verification
		verificationCopy.EvidenceRefs = append([]string(nil), verification.EvidenceRefs...)
		state.Verification = &verificationCopy
		if verification.DeterministicPassed && verification.EvaluatorPassed {
			state.Status = TaskStatusSucceeded
		} else {
			state.Status = TaskStatusFailed
		}
		return nil
	})
}

func (store *Store) Cancel(runID, taskID, message string) (TaskState, error) {
	return store.mutate(runID, taskID, EventTaskCancelled, func(state *TaskState, _ map[string]TaskState) error {
		if state.Status == TaskStatusCancelled {
			return errNoChange
		}
		if state.Status.Terminal() {
			return transitionError(state.Status, TaskStatusCancelled)
		}
		state.Status = TaskStatusCancelled
		state.Message = message
		return nil
	})
}

func (store *Store) RecoverExpiredLeases(runID string) ([]string, error) {
	if err := validateID("run", runID); err != nil {
		return nil, err
	}
	unlock, err := store.lockRun(runID)
	if err != nil {
		return nil, err
	}
	defer unlock()
	events, states, err := store.replayLocked(runID)
	if err != nil {
		return nil, err
	}
	taskIDs := make([]string, 0, len(states))
	for taskID := range states {
		taskIDs = append(taskIDs, taskID)
	}
	sort.Strings(taskIDs)

	interrupted := make([]string, 0)
	now := store.now()
	for _, taskID := range taskIDs {
		state := states[taskID]
		if state.Status != TaskStatusClaimed && state.Status != TaskStatusRunning {
			continue
		}
		if state.Lease == nil || now.Before(state.Lease.ExpiresAt) {
			continue
		}
		from := state.Status
		state.Status = TaskStatusInterrupted
		state.Message = "lease expired"
		state.Revision++
		state.UpdatedAt = now
		event := store.eventFor(events, state, from, EventTaskInterrupted)
		if err := store.commitEventLocked(event); err != nil {
			return interrupted, err
		}
		events = append(events, event)
		states[taskID] = state
		interrupted = append(interrupted, taskID)
	}
	return interrupted, nil
}

func (store *Store) mutate(runID, taskID string, eventType EventType, mutate mutation) (TaskState, error) {
	if err := validateID("run", runID); err != nil {
		return TaskState{}, err
	}
	if err := validateID("task", taskID); err != nil {
		return TaskState{}, err
	}
	unlock, err := store.lockRun(runID)
	if err != nil {
		return TaskState{}, err
	}
	defer unlock()
	events, states, err := store.replayLocked(runID)
	if err != nil {
		return TaskState{}, err
	}
	current, ok := states[taskID]
	if !ok {
		return TaskState{}, fmt.Errorf("%w: %s", ErrTaskNotFound, taskID)
	}
	next := cloneTaskState(current)
	if err := mutate(&next, states); err != nil {
		if errors.Is(err, errNoChange) {
			return current, nil
		}
		return TaskState{}, err
	}
	next.SchemaVersion = SchemaVersion
	next.Revision = current.Revision + 1
	next.UpdatedAt = store.now()
	event := store.eventFor(events, next, current.Status, eventType)
	if err := store.commitEventLocked(event); err != nil {
		return next, err
	}
	return next, nil
}

func (store *Store) validateLease(state TaskState, fencingToken string) error {
	if state.Lease == nil {
		return fmt.Errorf("task %s has no active lease", state.TaskID)
	}
	if fencingToken == "" || fencingToken != state.Lease.FencingToken {
		return fmt.Errorf("%w: task %s attempt %d", ErrStaleFence, state.TaskID, state.Attempt)
	}
	if !store.now().Before(state.Lease.ExpiresAt) {
		return fmt.Errorf("%w: task %s attempt %d", ErrLeaseExpired, state.TaskID, state.Attempt)
	}
	return nil
}

func (store *Store) token() (string, error) {
	if store.newToken == nil {
		return randomToken()
	}
	return store.newToken()
}

func transitionError(from, to TaskStatus) error {
	return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, from, to)
}

func cloneTaskState(state TaskState) TaskState {
	clone := state
	if state.Lease != nil {
		lease := *state.Lease
		clone.Lease = &lease
	}
	if state.Provider != nil {
		handle := *state.Provider
		clone.Provider = &handle
	}
	if state.Result != nil {
		result := cloneResult(*state.Result)
		clone.Result = &result
	}
	if state.Verification != nil {
		verification := *state.Verification
		verification.EvidenceRefs = append([]string(nil), state.Verification.EvidenceRefs...)
		clone.Verification = &verification
	}
	return clone
}

func cloneResult(result provider.TaskResult) provider.TaskResult {
	clone := result
	clone.ChangedFiles = append([]string(nil), result.ChangedFiles...)
	if result.Evidence != nil {
		clone.Evidence = make(map[string]string, len(result.Evidence))
		for key, value := range result.Evidence {
			clone.Evidence[key] = value
		}
	}
	return clone
}
