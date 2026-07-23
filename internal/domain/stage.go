// Package domain defines shared domain types used across multiple packages.
package domain

// Stage represents a pipeline execution stage inferred from runtime artifacts.
type Stage string

const (
	StageInit              Stage = "init"
	StagePOConversation    Stage = "po_conversation"
	StageArchitectAnalysis Stage = "architect_analysis"
	StagePMTaskBreakdown   Stage = "pm_task_breakdown"
	StageAgentExecuting    Stage = "agent_executing"
	StageVerification      Stage = "verification"
	StagePRCreation        Stage = "pr_creation"
)

// StageFromArtifacts returns the highest completed stage represented by the
// runtime artifact filenames.
func StageFromArtifacts(existingFiles []string) Stage {
	fileSet := make(map[string]bool, len(existingFiles))
	for _, file := range existingFiles {
		fileSet[file] = true
	}

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

	for _, artifact := range orderedArtifacts {
		if fileSet[artifact.file] {
			return artifact.stage
		}
	}
	return StageInit
}
