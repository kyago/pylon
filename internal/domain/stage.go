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

// PipelineStatus represents the operational status of a pipeline (orthogonal to Stage).
type PipelineStatus string

const (
	StatusRunning   PipelineStatus = "running"
	StatusPaused    PipelineStatus = "paused"
	StatusCompleted PipelineStatus = "completed"
	StatusFailed    PipelineStatus = "failed"
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

// ArtifactToStage maps a pipeline artifact filename to its corresponding stage.
// In v2, artifact existence in .pylon/runtime/{pipeline-id}/ indicates stage completion.
var ArtifactToStage = map[string]Stage{
	"requirement.md":          StageInit,
	"requirement-analysis.md": StagePOConversation,
	"architecture.md":         StageArchitectAnalysis,
	"tasks.json":              StagePMTaskBreakdown,
	"execution-log.json":      StageAgentExecuting,
	"verification.json":       StageVerification,
	"pr.json":                 StagePRCreation,
}

// StageFromArtifacts determines the current stage based on which artifacts exist.
// Returns the highest completed stage.
func StageFromArtifacts(existingFiles []string) Stage {
	fileSet := make(map[string]bool, len(existingFiles))
	for _, f := range existingFiles {
		fileSet[f] = true
	}

	// Check in reverse pipeline order
	orderedArtifacts := []struct {
		file  string
		stage Stage
	}{
		{"pr.json", StagePRCreation},
		{"verification.json", StageVerification},
		{"execution-log.json", StageAgentExecuting},
		{"tasks.json", StagePMTaskBreakdown},
		{"architecture.md", StageArchitectAnalysis},
		{"requirement-analysis.md", StagePOConversation},
		{"requirement.md", StageInit},
	}

	for _, a := range orderedArtifacts {
		if fileSet[a.file] {
			return a.stage
		}
	}
	return StageInit
}
