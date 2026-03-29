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

	// Generic stages for non-software domains (Harness architecture patterns)
	StageFanOut         Stage = "fan_out"          // Parallel branch (fan-out/fan-in pattern)
	StageFanIn          Stage = "fan_in"           // Parallel merge
	StageExpertSelect   Stage = "expert_select"    // Expert pool selection
	StageGenerate       Stage = "generate"         // Content/report generation
	StageValidate       Stage = "validate"         // Validation (generate-verify pattern)
	StageSupervisorCheck Stage = "supervisor_check" // Supervisor checkpoint
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
		// Generic stages for non-software domains
		StageFanOut,
		StageFanIn,
		StageExpertSelect,
		StageGenerate,
		StageValidate,
		StageSupervisorCheck,
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

// StageFromArtifactsWithMap determines the current stage based on which artifacts exist,
// using a custom artifact-to-stage mapping. If artifactMap is nil, falls back to the
// default ArtifactToStage map (backward compatible).
func StageFromArtifactsWithMap(existingFiles []string, artifactMap map[string]Stage) Stage {
	m := artifactMap
	if m == nil {
		m = ArtifactToStage
	}

	fileSet := make(map[string]bool, len(existingFiles))
	for _, f := range existingFiles {
		fileSet[f] = true
	}

	// Build ordered list from the map (reverse order by pipeline position)
	var lastStage Stage
	for file, stage := range m {
		if fileSet[file] {
			lastStage = stage
		}
	}

	if lastStage == "" {
		return StageInit
	}
	return lastStage
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
