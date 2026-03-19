// Package orchestrator implements the core pipeline and orchestration logic.
// Spec Reference: Section 7 "pylon request" execution flow, Section 8
package orchestrator

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/kyago/pylon/internal/domain"
)

// Stage is an alias for domain.Stage to maintain backward compatibility.
type Stage = domain.Stage

// Stage constants re-exported from domain package.
const (
	StageInit              = domain.StageInit
	StagePOConversation    = domain.StagePOConversation
	StageArchitectAnalysis = domain.StageArchitectAnalysis
	StagePMTaskBreakdown   = domain.StagePMTaskBreakdown
	StageTaskReview        = domain.StageTaskReview
	StageAgentExecuting    = domain.StageAgentExecuting
	StageVerification      = domain.StageVerification
	StagePRCreation        = domain.StagePRCreation
	StagePOValidation      = domain.StagePOValidation
	StageWikiUpdate        = domain.StageWikiUpdate
	StageCompleted         = domain.StageCompleted
	StageFailed            = domain.StageFailed
)

// validTransitions defines which stage transitions are allowed.
var validTransitions = map[Stage][]Stage{
	StageInit:              {StagePOConversation, StageFailed},
	StagePOConversation:    {StageArchitectAnalysis, StageFailed},
	StageArchitectAnalysis: {StagePMTaskBreakdown, StageFailed},
	StagePMTaskBreakdown:   {StageTaskReview, StageFailed},
	StageTaskReview:        {StageAgentExecuting, StageFailed},
	StageAgentExecuting:    {StageVerification, StageFailed},
	StageVerification:      {StageAgentExecuting, StagePRCreation, StageFailed}, // retry or proceed
	StagePRCreation:        {StagePOValidation, StageWikiUpdate, StageFailed},
	StagePOValidation:      {StageWikiUpdate, StageFailed},
	StageWikiUpdate:        {StageCompleted, StageFailed},
}

// Agent execution status constants.
const (
	AgentStatusRunning   = "running"
	AgentStatusCompleted = "completed"
	AgentStatusFailed    = "failed"
)

// AgentStatus tracks the state of an agent within a pipeline.
type AgentStatus struct {
	TaskID  string `json:"task_id"`
	AgentID string `json:"agent_id"`
	Status  string `json:"status"` // running, completed, failed
}

// StageTransition records a stage change event.
type StageTransition struct {
	From        Stage     `json:"from"`
	To          Stage     `json:"to"`
	CompletedAt time.Time `json:"completed_at"`
}

// PipelineStatus is an alias for domain.PipelineStatus.
type PipelineStatus = domain.PipelineStatus

// PipelineStatus constants re-exported from domain package.
const (
	StatusRunning   = domain.StatusRunning
	StatusPaused    = domain.StatusPaused
	StatusCompleted = domain.StatusCompleted
	StatusFailed    = domain.StatusFailed
)

// Pipeline represents the state of a single pylon request execution.
type Pipeline struct {
	ID            string                 `json:"pipeline_id"`
	CurrentStage  Stage                  `json:"current_stage"`
	WorkflowName  string                 `json:"workflow_name,omitempty"`
	Status        PipelineStatus         `json:"status"`
	PausedAtStage Stage                  `json:"paused_at_stage,omitempty"`
	TaskSpec      string                 `json:"task_spec,omitempty"`
	Agents        map[string]AgentStatus `json:"active_agents,omitempty"`
	History       []StageTransition      `json:"stage_history,omitempty"`
	Attempts      int                    `json:"attempts,omitempty"`
	MaxAttempts   int                    `json:"max_attempts,omitempty"`
	TaskGraph     *TaskGraph             `json:"task_graph,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`

	// transitions is a runtime-only field (not serialized) that overrides
	// the default validTransitions when a workflow template is applied.
	transitions map[Stage][]Stage `json:"-"`
}

// NewPipeline creates a new pipeline in the init stage.
func NewPipeline(id string, maxAttempts int) *Pipeline {
	if maxAttempts <= 0 {
		maxAttempts = 2
	}
	return &Pipeline{
		ID:           id,
		CurrentStage: StageInit,
		Status:       StatusRunning,
		Agents:       make(map[string]AgentStatus),
		MaxAttempts:  maxAttempts,
		CreatedAt:    time.Now(),
	}
}

// SetTransitions overrides the default transition map with a workflow-specific one.
// This is a runtime-only setting; the transitions are not serialized.
func (p *Pipeline) SetTransitions(t map[Stage][]Stage) {
	p.transitions = t
}

// getTransitions returns the active transition map.
// Returns the workflow-specific transitions if set, otherwise the default validTransitions.
func (p *Pipeline) getTransitions() map[Stage][]Stage {
	if p.transitions != nil {
		return p.transitions
	}
	return validTransitions
}

// CanTransition checks whether a transition to the target stage is valid.
func (p *Pipeline) CanTransition(to Stage) bool {
	allowed, ok := p.getTransitions()[p.CurrentStage]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}

// Transition moves the pipeline to a new stage if the transition is valid.
func (p *Pipeline) Transition(to Stage) error {
	if !p.CanTransition(to) {
		return fmt.Errorf("invalid transition: %s → %s", p.CurrentStage, to)
	}

	// Track retry attempts for verification → agent_executing loops
	if p.CurrentStage == StageVerification && to == StageAgentExecuting {
		if p.Attempts >= p.MaxAttempts {
			return fmt.Errorf("max retry attempts (%d) reached", p.MaxAttempts)
		}
		p.Attempts++
	}

	p.History = append(p.History, StageTransition{
		From:        p.CurrentStage,
		To:          to,
		CompletedAt: time.Now(),
	})
	p.CurrentStage = to
	return nil
}

// Snapshot serializes the pipeline state to JSON.
func (p *Pipeline) Snapshot() ([]byte, error) {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to snapshot pipeline: %w", err)
	}
	return data, nil
}

// LoadPipeline deserializes a pipeline from JSON.
func LoadPipeline(data []byte) (*Pipeline, error) {
	p := &Pipeline{}
	if err := json.Unmarshal(data, p); err != nil {
		return nil, fmt.Errorf("failed to load pipeline: %w", err)
	}
	return p, nil
}

// IsTerminal returns true if the pipeline is in a terminal state.
func (p *Pipeline) IsTerminal() bool {
	return p.CurrentStage == StageCompleted || p.CurrentStage == StageFailed
}

// IsPaused returns true if the pipeline status is paused.
func (p *Pipeline) IsPaused() bool {
	return p.Status == StatusPaused
}

// Pause sets the pipeline status to paused and records the current stage.
func (p *Pipeline) Pause() error {
	if p.IsTerminal() {
		return fmt.Errorf("cannot pause terminal pipeline")
	}
	if p.IsPaused() {
		return fmt.Errorf("pipeline already paused")
	}
	p.Status = StatusPaused
	p.PausedAtStage = p.CurrentStage
	return nil
}

// Resume restores the pipeline status to running and clears the paused stage.
func (p *Pipeline) Resume() error {
	if !p.IsPaused() {
		return fmt.Errorf("pipeline is not paused")
	}
	p.Status = StatusRunning
	p.PausedAtStage = ""
	return nil
}
