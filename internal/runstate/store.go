package runstate

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/kyago/pylon/internal/fsutil"
	"github.com/kyago/pylon/internal/layout"
)

type Store struct {
	Root        string
	Now         func() time.Time
	LockTimeout time.Duration

	newToken  func() (string, error)
	writeJSON func(string, any) error
}

func NewStore(root string) *Store {
	return &Store{
		Root:        root,
		Now:         time.Now,
		LockTimeout: 10 * time.Second,
		newToken:    randomToken,
		writeJSON:   fsutil.WriteJSONAtomic,
	}
}

func (store *Store) CreateRun(spec RunSpec) (RunState, error) {
	if err := validateID("run", spec.RunID); err != nil {
		return RunState{}, err
	}
	runDir := store.runDir(spec.RunID)
	if err := os.MkdirAll(runDir, 0755); err != nil {
		return RunState{}, err
	}
	unlock, err := store.lockRun(spec.RunID)
	if err != nil {
		return RunState{}, err
	}
	defer unlock()

	path := filepath.Join(runDir, "run.json")
	if existing, err := readJSON[RunState](path); err == nil {
		if existing.RunID == spec.RunID && existing.Requirement == spec.Requirement {
			return existing, nil
		}
		return RunState{}, fmt.Errorf("%w: run %s", ErrAlreadyExists, spec.RunID)
	} else if !os.IsNotExist(err) {
		return RunState{}, err
	}

	now := store.now()
	state := RunState{
		SchemaVersion: SchemaVersion,
		RunID:         spec.RunID,
		Requirement:   spec.Requirement,
		Status:        RunStatusActive,
		Revision:      1,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := store.writeJSON(path, state); err != nil {
		return RunState{}, err
	}
	return state, nil
}

func (store *Store) CreateTask(spec TaskSpec) (TaskState, error) {
	if err := validateID("run", spec.RunID); err != nil {
		return TaskState{}, err
	}
	if err := validateID("task", spec.TaskID); err != nil {
		return TaskState{}, err
	}
	if err := store.ensureRun(spec.RunID); err != nil {
		return TaskState{}, err
	}
	unlock, err := store.lockRun(spec.RunID)
	if err != nil {
		return TaskState{}, err
	}
	defer unlock()

	events, states, err := store.replayLocked(spec.RunID)
	if err != nil {
		return TaskState{}, err
	}
	if existing, ok := states[spec.TaskID]; ok {
		storedSpec, readErr := readJSON[TaskSpec](store.taskSpecPath(spec.RunID, spec.TaskID))
		if readErr == nil && reflect.DeepEqual(storedSpec, normalizeTaskSpec(spec)) {
			return existing, nil
		}
		return TaskState{}, fmt.Errorf("%w: task %s", ErrAlreadyExists, spec.TaskID)
	}

	spec = normalizeTaskSpec(spec)
	if err := store.writeJSON(store.taskSpecPath(spec.RunID, spec.TaskID), spec); err != nil {
		return TaskState{}, err
	}
	now := store.now()
	state := TaskState{
		SchemaVersion: SchemaVersion,
		RunID:         spec.RunID,
		TaskID:        spec.TaskID,
		Status:        TaskStatusPending,
		Revision:      1,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	event := store.eventFor(events, state, "", EventTaskCreated)
	if err := store.commitEventLocked(event); err != nil {
		return TaskState{}, err
	}
	return state, nil
}

func (store *Store) ReadTask(runID, taskID string) (TaskState, error) {
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
	_, states, err := store.replayLocked(runID)
	if err != nil {
		return TaskState{}, err
	}
	state, ok := states[taskID]
	if !ok {
		return TaskState{}, fmt.Errorf("%w: %s", ErrTaskNotFound, taskID)
	}
	if _, repaired, err := store.materializeIfNeededLocked(state); err != nil {
		return TaskState{}, err
	} else if repaired {
		return state, nil
	}
	return state, nil
}

func (store *Store) ReadEvents(runID string) ([]Event, error) {
	if err := validateID("run", runID); err != nil {
		return nil, err
	}
	unlock, err := store.lockRun(runID)
	if err != nil {
		return nil, err
	}
	defer unlock()
	events, _, err := store.replayLocked(runID)
	return events, err
}

func (store *Store) Recover(runID string) ([]string, error) {
	if err := validateID("run", runID); err != nil {
		return nil, err
	}
	unlock, err := store.lockRun(runID)
	if err != nil {
		return nil, err
	}
	defer unlock()
	_, states, err := store.replayLocked(runID)
	if err != nil {
		return nil, err
	}
	repaired := make([]string, 0)
	for taskID, state := range states {
		_, changed, err := store.materializeIfNeededLocked(state)
		if err != nil {
			return nil, err
		}
		if changed {
			repaired = append(repaired, taskID)
		}
	}
	sort.Strings(repaired)
	return repaired, nil
}

func (store *Store) replayLocked(runID string) ([]Event, map[string]TaskState, error) {
	if err := store.ensureRun(runID); err != nil {
		return nil, nil, err
	}
	events, err := store.readEventsLocked(runID)
	if err != nil {
		return nil, nil, err
	}
	states := make(map[string]TaskState)
	for index, event := range events {
		expectedSequence := uint64(index + 1)
		if event.Sequence != expectedSequence {
			return nil, nil, fmt.Errorf("event sequence gap: got %d, want %d", event.Sequence, expectedSequence)
		}
		if event.RunID != runID || event.State.RunID != runID || event.TaskID != event.State.TaskID {
			return nil, nil, fmt.Errorf("event identity mismatch at sequence %d", event.Sequence)
		}
		if event.Revision != event.State.Revision || event.To != event.State.Status {
			return nil, nil, fmt.Errorf("event state mismatch at sequence %d", event.Sequence)
		}
		if previous, ok := states[event.TaskID]; ok {
			if event.Revision != previous.Revision+1 || event.From != previous.Status {
				return nil, nil, fmt.Errorf("task %s event revision mismatch at sequence %d", event.TaskID, event.Sequence)
			}
		} else if event.Revision != 1 || event.Type != EventTaskCreated {
			return nil, nil, fmt.Errorf("task %s does not begin with task_created", event.TaskID)
		}
		states[event.TaskID] = event.State
	}
	return events, states, nil
}

func (store *Store) readEventsLocked(runID string) ([]Event, error) {
	path := store.eventsPath(runID)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	if data[len(data)-1] != '\n' {
		lastNewline := bytes.LastIndexByte(data, '\n')
		committedLength := int64(lastNewline + 1)
		if err := os.Truncate(path, committedLength); err != nil {
			return nil, err
		}
		data = data[:committedLength]
	}
	lines := bytes.Split(data, []byte{'\n'})
	events := make([]Event, 0, len(lines)-1)
	for lineNumber, line := range lines {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var event Event
		if err := json.Unmarshal(line, &event); err != nil {
			return nil, fmt.Errorf("events.jsonl line %d: %w", lineNumber+1, err)
		}
		events = append(events, event)
	}
	return events, nil
}

func (store *Store) eventFor(events []Event, state TaskState, from TaskStatus, eventType EventType) Event {
	sequence := uint64(len(events) + 1)
	return Event{
		SchemaVersion: SchemaVersion,
		Sequence:      sequence,
		EventID:       fmt.Sprintf("%s:%020d", state.RunID, sequence),
		RunID:         state.RunID,
		TaskID:        state.TaskID,
		Type:          eventType,
		From:          from,
		To:            state.Status,
		Revision:      state.Revision,
		Attempt:       state.Attempt,
		RecordedAt:    state.UpdatedAt,
		State:         state,
	}
}

func (store *Store) commitEventLocked(event Event) error {
	if err := appendEvent(store.eventsPath(event.RunID), event); err != nil {
		return err
	}
	return store.materializeTaskLocked(event.State)
}

func (store *Store) materializeIfNeededLocked(state TaskState) (TaskState, bool, error) {
	current, err := readJSON[TaskState](store.taskStatePath(state.RunID, state.TaskID))
	if err == nil && reflect.DeepEqual(current, state) {
		return current, false, nil
	}
	if err != nil && !os.IsNotExist(err) {
		var pathError *os.PathError
		if errors.As(err, &pathError) {
			return TaskState{}, false, err
		}
	}
	if err := store.materializeTaskLocked(state); err != nil {
		return TaskState{}, false, err
	}
	return state, true, nil
}

func (store *Store) materializeTaskLocked(state TaskState) error {
	if err := store.writeJSON(store.taskStatePath(state.RunID, state.TaskID), state); err != nil {
		return err
	}
	if state.Attempt == 0 {
		return nil
	}
	attemptDir := store.attemptDir(state.RunID, state.TaskID, state.Attempt)
	if state.Lease != nil {
		if err := store.writeJSON(filepath.Join(attemptDir, "lease.json"), state.Lease); err != nil {
			return err
		}
	}
	if state.Provider != nil {
		if err := store.writeJSON(filepath.Join(attemptDir, "provider.json"), state.Provider); err != nil {
			return err
		}
	}
	if state.Result != nil {
		if err := store.writeJSON(filepath.Join(attemptDir, "result.json"), state.Result); err != nil {
			return err
		}
	}
	return nil
}

func appendEvent(path string, event Event) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	written, writeErr := file.Write(data)
	if writeErr == nil && written != len(data) {
		writeErr = fmt.Errorf("short event write: %d of %d bytes", written, len(data))
	}
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

func readJSON[T any](path string) (T, error) {
	var value T
	data, err := os.ReadFile(path)
	if err != nil {
		return value, err
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return value, err
	}
	return value, nil
}

func normalizeTaskSpec(spec TaskSpec) TaskSpec {
	spec.SchemaVersion = SchemaVersion
	if len(spec.Dependencies) == 0 {
		spec.Dependencies = nil
	}
	if len(spec.AcceptanceCriteria) == 0 {
		spec.AcceptanceCriteria = nil
	}
	return spec
}

func (store *Store) ensureRun(runID string) error {
	if _, err := os.Stat(filepath.Join(store.runDir(runID), "run.json")); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %s", ErrRunNotFound, runID)
		}
		return err
	}
	return nil
}

func (store *Store) lockRun(runID string) (func(), error) {
	if err := os.MkdirAll(store.runDir(runID), 0755); err != nil {
		return nil, err
	}
	return fsutil.AcquireFileLock(filepath.Join(store.runDir(runID), ".state.lock"), store.LockTimeout)
}

func (store *Store) now() time.Time {
	if store.Now == nil {
		return time.Now().UTC()
	}
	return store.Now().UTC()
}

func (store *Store) runDir(runID string) string {
	return filepath.Join(layout.RuntimeDir(store.Root), runID)
}

func (store *Store) eventsPath(runID string) string {
	return filepath.Join(store.runDir(runID), "events.jsonl")
}

func (store *Store) taskDir(runID, taskID string) string {
	return filepath.Join(store.runDir(runID), "tasks", taskID)
}

func (store *Store) taskSpecPath(runID, taskID string) string {
	return filepath.Join(store.taskDir(runID, taskID), "spec.json")
}

func (store *Store) taskStatePath(runID, taskID string) string {
	return filepath.Join(store.taskDir(runID, taskID), "state.json")
}

func (store *Store) attemptDir(runID, taskID string, attempt int) string {
	return filepath.Join(store.taskDir(runID, taskID), "attempts", fmt.Sprintf("%03d", attempt))
}

func validateID(kind, value string) error {
	if value == "" || value == "." || value == ".." || filepath.Base(value) != value || strings.ContainsAny(value, `/\\`) {
		return fmt.Errorf("invalid %s id: %q", kind, value)
	}
	return nil
}

func randomToken() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}
