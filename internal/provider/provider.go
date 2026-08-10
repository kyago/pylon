package provider

import (
	"context"
	"errors"
	"time"
)

var ErrUnsupportedOperation = errors.New("provider operation is not supported")

type Criterion struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

type TaskSpec struct {
	RunID                string        `json:"run_id"`
	TaskID               string        `json:"task_id"`
	RepoID               string        `json:"repo_id"`
	WorktreePath         string        `json:"worktree_path"`
	BaseRevision         string        `json:"base_revision"`
	AcceptanceCriteria   []Criterion   `json:"acceptance_criteria"`
	RequiredCapabilities CapabilitySet `json:"required_capabilities"`
	ArtifactDir          string        `json:"artifact_dir"`
}

type WorkerHandle struct {
	Provider    string `json:"provider"`
	ExternalID  string `json:"external_id"`
	ResumeToken string `json:"resume_token,omitempty"`
	Attempt     int    `json:"attempt"`
}

type WorkerState string

const (
	WorkerStatePending     WorkerState = "pending"
	WorkerStateRunning     WorkerState = "running"
	WorkerStateSucceeded   WorkerState = "succeeded"
	WorkerStateFailed      WorkerState = "failed"
	WorkerStateInterrupted WorkerState = "interrupted"
	WorkerStateCancelled   WorkerState = "cancelled"
)

type WorkerStatus struct {
	State     WorkerState `json:"state"`
	Message   string      `json:"message,omitempty"`
	UpdatedAt time.Time   `json:"updated_at"`
}

type TaskResult struct {
	State        WorkerState       `json:"state"`
	ExitCode     int               `json:"exit_code"`
	ChangedFiles []string          `json:"changed_files,omitempty"`
	Evidence     map[string]string `json:"evidence,omitempty"`
}

type Adapter interface {
	Name() string
	Probe(context.Context) (CapabilitySet, error)
	Start(context.Context, TaskSpec) (WorkerHandle, error)
	Poll(context.Context, WorkerHandle) (WorkerStatus, error)
	Resume(context.Context, WorkerHandle) (WorkerHandle, error)
	Cancel(context.Context, WorkerHandle) error
	Collect(context.Context, WorkerHandle) (TaskResult, error)
}
