package domain

import "testing"

func TestStageFromArtifacts_ReturnsHighestCompletedStage(t *testing.T) {
	tests := []struct {
		name  string
		files []string
		want  Stage
	}{
		{name: "empty", want: StageInit},
		{name: "analysis", files: []string{"requirement.md", "requirement-analysis.md"}, want: StagePOConversation},
		{name: "highest wins", files: []string{"tasks.json", "verification.json", "architecture.md"}, want: StageVerification},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StageFromArtifacts(tt.files); got != tt.want {
				t.Fatalf("StageFromArtifacts(%v) = %q, want %q", tt.files, got, tt.want)
			}
		})
	}
}
