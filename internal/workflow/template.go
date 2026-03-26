// Package workflow defines workflow templates that control which pipeline stages
// are executed and in what order, enabling adaptive workflows per task type.
package workflow

import (
	"github.com/kyago/pylon/internal/domain"
)

// Stage is an alias for domain.Stage.
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

// LoopDef defines an additional non-linear transition (e.g., loopback or skip).
type LoopDef struct {
	From Stage `yaml:"from"`
	To   Stage `yaml:"to"`
}

// WorkflowTemplate defines a pipeline execution path.
type WorkflowTemplate struct {
	Name              string    `yaml:"name"`
	Description       string    `yaml:"description"`
	Stages            []Stage   `yaml:"stages"`
	Loops             []LoopDef `yaml:"loops,omitempty"`
	AllowDynamicSpawn bool      `yaml:"allow_dynamic_spawn,omitempty"` // Reserved for Phase 5
}

// BuildTransitions generates a transition map from the workflow's stage sequence.
//
// Algorithm:
//  1. Linear chain: stage[i] → stage[i+1]
//  2. All non-terminal stages → StageFailed
//  3. Auto-infer: if StageVerification is present, add loopback to the preceding stage
//  4. Add explicit transitions from Loops
//  5. If autoPR is true, inject StagePRCreation as an alternative target from StageVerification
//
// The autoPR parameter allows opt-in PR creation even when the workflow template
// does not include pr_creation in its stage list. When true, StageVerification gains
// an additional transition to StagePRCreation, and StagePRCreation gains transitions
// to the stage that follows StageVerification in the template plus StageFailed.
func (w *WorkflowTemplate) BuildTransitions(autoPR ...bool) map[Stage][]Stage {
	transitions := make(map[Stage][]Stage)

	// 1. Linear chain
	for i := 0; i < len(w.Stages)-1; i++ {
		transitions[w.Stages[i]] = append(transitions[w.Stages[i]], w.Stages[i+1])
	}

	// 2. All non-terminal stages → failed
	for _, s := range w.Stages {
		if s != StageCompleted && s != StageFailed {
			transitions[s] = appendUnique(transitions[s], StageFailed)
		}
	}

	// 3. Auto-infer verification loopback (미결 #6 해소)
	for i, s := range w.Stages {
		if s == StageVerification && i > 0 {
			prevStage := w.Stages[i-1]
			transitions[s] = appendUnique(transitions[s], prevStage)
		}
	}

	// 4. Explicit loops / extra transitions
	for _, loop := range w.Loops {
		transitions[loop.From] = appendUnique(transitions[loop.From], loop.To)
	}

	// 5. Auto-inject pr_creation when autoPR is enabled
	if len(autoPR) > 0 && autoPR[0] && !w.HasStage(StagePRCreation) {
		for i, s := range w.Stages {
			if s == StageVerification && i+1 < len(w.Stages) {
				nextStage := w.Stages[i+1]
				transitions[StageVerification] = appendUnique(transitions[StageVerification], StagePRCreation)
				transitions[StagePRCreation] = []Stage{nextStage, StageCompleted, StageFailed}
				break
			}
		}
	}

	return transitions
}

// HasStage returns true if the given stage is part of this workflow.
func (w *WorkflowTemplate) HasStage(stage Stage) bool {
	for _, s := range w.Stages {
		if s == stage {
			return true
		}
	}
	return false
}

// NextStageAfter returns the next stage in the workflow after the given stage.
// Returns StageFailed if the stage is not found or is the last stage.
func (w *WorkflowTemplate) NextStageAfter(current Stage) Stage {
	for i, s := range w.Stages {
		if s == current && i+1 < len(w.Stages) {
			return w.Stages[i+1]
		}
	}
	return StageFailed
}

// containsStage returns true if stages contains target.
func containsStage(stages []Stage, target Stage) bool {
	for _, s := range stages {
		if s == target {
			return true
		}
	}
	return false
}

func appendUnique(stages []Stage, target Stage) []Stage {
	if containsStage(stages, target) {
		return stages
	}
	return append(stages, target)
}
