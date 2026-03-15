package domain

import "testing"

func TestAllStages_ContainsAllConstants(t *testing.T) {
	// 모든 Stage 상수가 AllStages()에 포함되어야 합니다.
	expected := []Stage{
		StageInit,
		StagePOConversation,
		StageArchitectAnalysis,
		StagePMTaskBreakdown,
		StageAgentExecuting,
		StageVerification,
		StagePRCreation,
		StagePOValidation,
		StageWikiUpdate,
		StageCompleted,
		StageFailed,
	}

	all := AllStages()
	if len(all) != len(expected) {
		t.Fatalf("AllStages() returned %d stages, expected %d", len(all), len(expected))
	}

	stageSet := make(map[Stage]bool)
	for _, s := range all {
		stageSet[s] = true
	}

	for _, s := range expected {
		if !stageSet[s] {
			t.Errorf("AllStages() is missing stage %q", s)
		}
	}
}

func TestAllStages_NoDuplicates(t *testing.T) {
	all := AllStages()
	seen := make(map[Stage]bool)
	for _, s := range all {
		if seen[s] {
			t.Errorf("AllStages() contains duplicate stage %q", s)
		}
		seen[s] = true
	}
}

func TestAllStages_NoEmpty(t *testing.T) {
	for _, s := range AllStages() {
		if s == "" {
			t.Error("AllStages() contains empty stage")
		}
	}
}
