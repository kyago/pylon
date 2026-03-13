// Package orchestrator implements the core pipeline and orchestration logic.
// Spec Reference: Section 7 "pylon request" execution flow, Section 8
package orchestrator

import (
	"encoding/json"
	"fmt"
	"time"
)

// Stage represents a pipeline execution stage.
type Stage string

const (
	StageInit              Stage = "init"
	StagePOConversation    Stage = "po_conversation"
	StageArchitectAnalysis Stage = "architect_analysis"
	StagePMTaskBreakdown   Stage = "pm_task_breakdown"
	StageAgentExecuting    Stage = "agent_executing"
	StageVerification      Stage = "verification"
	StagePRCreation        Stage = "pr_creation"
	StagePOValidation      Stage = "po_validation"
	StageWikiUpdate        Stage = "wiki_update"
	StageCompleted         Stage = "completed"
	StageFailed            Stage = "failed"
)

// validTransitions defines which stage transitions are allowed.
var validTransitions = map[Stage][]Stage{
	StageInit:              {StagePOConversation, StageFailed},
	StagePOConversation:    {StageArchitectAnalysis, StageFailed},
	StageArchitectAnalysis: {StagePMTaskBreakdown, StageFailed},
	StagePMTaskBreakdown:   {StageAgentExecuting, StageFailed},
	StageAgentExecuting:    {StageVerification, StageFailed},
	StageVerification:      {StageAgentExecuting, StagePRCreation, StageFailed}, // retry or proceed
	StagePRCreation:        {StagePOValidation, StageFailed},
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

// Pipeline represents the state of a single pylon request execution.
type Pipeline struct {
	ID           string                 `json:"pipeline_id"`
	CurrentStage Stage                  `json:"current_stage"`
	TaskSpec     string                 `json:"task_spec,omitempty"`
	Agents       map[string]AgentStatus `json:"active_agents,omitempty"`
	History      []StageTransition      `json:"stage_history,omitempty"`
	Attempts     int                    `json:"attempts,omitempty"`
	MaxAttempts  int                    `json:"max_attempts,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
}

// NewPipeline creates a new pipeline in the init stage.
func NewPipeline(id string, maxAttempts int) *Pipeline {
	if maxAttempts <= 0 {
		maxAttempts = 2
	}
	return &Pipeline{
		ID:           id,
		CurrentStage: StageInit,
		Agents:       make(map[string]AgentStatus),
		MaxAttempts:  maxAttempts,
		CreatedAt:    time.Now(),
	}
}

// CanTransition checks whether a transition to the target stage is valid.
func (p *Pipeline) CanTransition(to Stage) bool {
	allowed, ok := validTransitions[p.CurrentStage]
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
