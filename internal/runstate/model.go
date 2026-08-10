// Package runstate persists durable run and task state transitions.
package runstate

import (
	"errors"
	"time"

	"github.com/kyago/pylon/internal/provider"
)

const SchemaVersion = 1

var (
	ErrRunNotFound       = errors.New("run not found")
	ErrTaskNotFound      = errors.New("task not found")
	ErrAlreadyExists     = errors.New("state already exists")
	ErrInvalidTransition = errors.New("invalid task state transition")
	ErrDependencyPending = errors.New("task dependency is not succeeded")
	ErrStaleFence        = errors.New("stale attempt fencing token")
	ErrLeaseExpired      = errors.New("task lease expired")
)

type RunStatus string

const (
	RunStatusActive    RunStatus = "active"
	RunStatusSucceeded RunStatus = "succeeded"
	RunStatusFailed    RunStatus = "failed"
	RunStatusCancelled RunStatus = "cancelled"
)

type TaskStatus string

const (
	TaskStatusPending     TaskStatus = "pending"
	TaskStatusReady       TaskStatus = "ready"
	TaskStatusClaimed     TaskStatus = "claimed"
	TaskStatusRunning     TaskStatus = "running"
	TaskStatusBlocked     TaskStatus = "blocked"
	TaskStatusVerifying   TaskStatus = "verifying"
	TaskStatusSucceeded   TaskStatus = "succeeded"
	TaskStatusFailed      TaskStatus = "failed"
	TaskStatusInterrupted TaskStatus = "interrupted"
	TaskStatusCancelled   TaskStatus = "cancelled"
)

func (status TaskStatus) Terminal() bool {
	switch status {
	case TaskStatusSucceeded, TaskStatusFailed, TaskStatusCancelled:
		return true
	default:
		return false
	}
}

type RunSpec struct {
	RunID       string `json:"run_id"`
	Requirement string `json:"requirement,omitempty"`
}

type RunState struct {
	SchemaVersion int       `json:"schema_version"`
	RunID         string    `json:"run_id"`
	Requirement   string    `json:"requirement,omitempty"`
	Status        RunStatus `json:"status"`
	Revision      uint64    `json:"revision"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type TaskSpec struct {
	SchemaVersion        int                    `json:"schema_version"`
	RunID                string                 `json:"run_id"`
	TaskID               string                 `json:"task_id"`
	RepoID               string                 `json:"repo_id,omitempty"`
	WorktreePath         string                 `json:"worktree_path,omitempty"`
	BaseRevision         string                 `json:"base_revision,omitempty"`
	AcceptanceCriteria   []provider.Criterion   `json:"acceptance_criteria,omitempty"`
	RequiredCapabilities provider.CapabilitySet `json:"required_capabilities,omitempty"`
	ArtifactDir          string                 `json:"artifact_dir,omitempty"`
	Dependencies         []string               `json:"dependencies,omitempty"`
}

type Lease struct {
	Owner        string    `json:"owner"`
	Attempt      int       `json:"attempt"`
	FencingToken string    `json:"fencing_token"`
	HeartbeatAt  time.Time `json:"heartbeat_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type Verification struct {
	DeterministicPassed bool      `json:"deterministic_passed"`
	EvaluatorPassed     bool      `json:"evaluator_passed"`
	EvidenceRefs        []string  `json:"evidence_refs,omitempty"`
	RecordedAt          time.Time `json:"recorded_at"`
}

type TaskState struct {
	SchemaVersion int                    `json:"schema_version"`
	RunID         string                 `json:"run_id"`
	TaskID        string                 `json:"task_id"`
	Status        TaskStatus             `json:"status"`
	Revision      uint64                 `json:"revision"`
	Attempt       int                    `json:"attempt"`
	Lease         *Lease                 `json:"lease,omitempty"`
	Provider      *provider.WorkerHandle `json:"provider,omitempty"`
	Result        *provider.TaskResult   `json:"result,omitempty"`
	Verification  *Verification          `json:"verification,omitempty"`
	Message       string                 `json:"message,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
}

type EventType string

const (
	EventTaskCreated      EventType = "task_created"
	EventTaskReady        EventType = "task_ready"
	EventTaskClaimed      EventType = "task_claimed"
	EventTaskStarted      EventType = "task_started"
	EventTaskHeartbeat    EventType = "task_heartbeat"
	EventTaskInterrupted  EventType = "task_interrupted"
	EventTaskResumed      EventType = "task_resumed"
	EventTaskRetried      EventType = "task_retried"
	EventTaskWorkReported EventType = "task_work_reported"
	EventTaskVerified     EventType = "task_verified"
	EventTaskBlocked      EventType = "task_blocked"
	EventTaskCancelled    EventType = "task_cancelled"
)

type Event struct {
	SchemaVersion int        `json:"schema_version"`
	Sequence      uint64     `json:"sequence"`
	EventID       string     `json:"event_id"`
	RunID         string     `json:"run_id"`
	TaskID        string     `json:"task_id"`
	Type          EventType  `json:"type"`
	From          TaskStatus `json:"from,omitempty"`
	To            TaskStatus `json:"to"`
	Revision      uint64     `json:"revision"`
	Attempt       int        `json:"attempt"`
	RecordedAt    time.Time  `json:"recorded_at"`
	State         TaskState  `json:"state"`
}
