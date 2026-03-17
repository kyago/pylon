// Package domain defines shared domain types used across multiple packages.
// Stage constants are the single source of truth for pipeline stages,
// eliminating manual synchronization between orchestrator and store.
package domain

// Stage represents a pipeline execution stage.
type Stage string

const (
	StageInit              Stage = "init"
	StagePOConversation    Stage = "po_conversation"
	StageArchitectAnalysis Stage = "architect_analysis"
	StagePMTaskBreakdown   Stage = "pm_task_breakdown"
	StageTaskReview        Stage = "task_review"
	StageAgentExecuting    Stage = "agent_executing"
	StageVerification      Stage = "verification"
	StagePRCreation        Stage = "pr_creation"
	StagePOValidation      Stage = "po_validation"
	StageWikiUpdate        Stage = "wiki_update"
	StageCompleted         Stage = "completed"
	StageFailed            Stage = "failed"
)

// AllStages returns all valid pipeline stages.
func AllStages() []Stage {
	return []Stage{
		StageInit,
		StagePOConversation,
		StageArchitectAnalysis,
		StagePMTaskBreakdown,
		StageTaskReview,
		StageAgentExecuting,
		StageVerification,
		StagePRCreation,
		StagePOValidation,
		StageWikiUpdate,
		StageCompleted,
		StageFailed,
	}
}
