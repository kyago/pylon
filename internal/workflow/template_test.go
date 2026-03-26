package workflow

import (
	"sort"
	"testing"
)

func TestBuildTransitions_Feature(t *testing.T) {
	wf := &WorkflowTemplate{
		Name: "feature",
		Stages: []Stage{
			StageInit, StagePOConversation, StageArchitectAnalysis,
			StagePMTaskBreakdown, StageTaskReview, StageAgentExecuting,
			StageVerification, StagePOValidation,
			StageWikiUpdate, StageCompleted,
		},
	}

	got := wf.BuildTransitions()

	// Expected transitions (pr_creation removed from default flow)
	expected := map[Stage][]Stage{
		StageInit:              {StagePOConversation, StageFailed},
		StagePOConversation:    {StageArchitectAnalysis, StageFailed},
		StageArchitectAnalysis: {StagePMTaskBreakdown, StageFailed},
		StagePMTaskBreakdown:   {StageTaskReview, StageFailed},
		StageTaskReview:        {StageAgentExecuting, StageFailed},
		StageAgentExecuting:    {StageVerification, StageFailed},
		StageVerification:      {StagePOValidation, StageFailed, StageAgentExecuting},
		StagePOValidation:      {StageWikiUpdate, StageFailed},
		StageWikiUpdate:        {StageCompleted, StageFailed},
	}

	if len(got) != len(expected) {
		t.Fatalf("expected %d stages in transitions, got %d", len(expected), len(got))
	}

	for stage, expectedTargets := range expected {
		gotTargets, ok := got[stage]
		if !ok {
			t.Errorf("missing transitions for stage %q", stage)
			continue
		}
		if !stageSetEqual(gotTargets, expectedTargets) {
			t.Errorf("stage %q: expected targets %v, got %v", stage, expectedTargets, gotTargets)
		}
	}
}

func TestBuildTransitions_Bugfix(t *testing.T) {
	wf := &WorkflowTemplate{
		Name:   "bugfix",
		Stages: []Stage{StageInit, StagePOConversation, StageAgentExecuting, StageVerification, StageCompleted},
	}

	got := wf.BuildTransitions()

	// Verification should auto-loopback to agent_executing
	verTargets := got[StageVerification]
	if !containsStage(verTargets, StageAgentExecuting) {
		t.Errorf("bugfix verification should loopback to agent_executing, got %v", verTargets)
	}
	if !containsStage(verTargets, StageCompleted) {
		t.Errorf("bugfix verification should forward to completed, got %v", verTargets)
	}

	// PO should go to agent_executing (skipping architect/PM/task_review)
	poTargets := got[StagePOConversation]
	if !containsStage(poTargets, StageAgentExecuting) {
		t.Errorf("bugfix po_conversation should go to agent_executing, got %v", poTargets)
	}
	if containsStage(poTargets, StageArchitectAnalysis) {
		t.Errorf("bugfix po_conversation should NOT go to architect_analysis, got %v", poTargets)
	}
}

func TestBuildTransitions_AutoPRInjection(t *testing.T) {
	wf := &WorkflowTemplate{
		Name:   "bugfix",
		Stages: []Stage{StageInit, StagePOConversation, StageAgentExecuting, StageVerification, StageCompleted},
	}

	// Without autoPR: verification should NOT have pr_creation
	got := wf.BuildTransitions()
	if containsStage(got[StageVerification], StagePRCreation) {
		t.Errorf("without autoPR, verification should not transition to pr_creation, got %v", got[StageVerification])
	}

	// With autoPR: verification should have pr_creation as additional target
	gotPR := wf.BuildTransitions(true)
	if !containsStage(gotPR[StageVerification], StagePRCreation) {
		t.Errorf("with autoPR, verification should transition to pr_creation, got %v", gotPR[StageVerification])
	}
	// pr_creation should transition to the next stage (completed) and failed
	prTargets := gotPR[StagePRCreation]
	if !containsStage(prTargets, StageCompleted) {
		t.Errorf("pr_creation should transition to completed, got %v", prTargets)
	}
	if !containsStage(prTargets, StageFailed) {
		t.Errorf("pr_creation should transition to failed, got %v", prTargets)
	}
}

func TestBuildTransitions_AutoVerificationLoopback(t *testing.T) {
	wf := &WorkflowTemplate{
		Name:   "test",
		Stages: []Stage{StageInit, StageAgentExecuting, StageVerification, StageCompleted},
	}

	got := wf.BuildTransitions()
	verTargets := got[StageVerification]

	if !containsStage(verTargets, StageAgentExecuting) {
		t.Errorf("verification should auto-loopback to preceding stage (agent_executing), got %v", verTargets)
	}
}

func TestBuildTransitions_NoVerification(t *testing.T) {
	wf := &WorkflowTemplate{
		Name:   "explore",
		Stages: []Stage{StageInit, StagePOConversation, StageArchitectAnalysis, StageCompleted},
	}

	got := wf.BuildTransitions()

	// No verification stage → no loopback
	if _, ok := got[StageVerification]; ok {
		t.Error("explore workflow should not have verification transitions")
	}
}

func TestBuildTransitions_FailedFromAllStages(t *testing.T) {
	wf := &WorkflowTemplate{
		Name:   "test",
		Stages: []Stage{StageInit, StagePOConversation, StageAgentExecuting, StageCompleted},
	}

	got := wf.BuildTransitions()
	nonTerminal := []Stage{StageInit, StagePOConversation, StageAgentExecuting}

	for _, s := range nonTerminal {
		if !containsStage(got[s], StageFailed) {
			t.Errorf("stage %q should allow transition to failed", s)
		}
	}
}

func TestNextStageAfter(t *testing.T) {
	wf := &WorkflowTemplate{
		Stages: []Stage{StageInit, StagePOConversation, StageAgentExecuting, StageCompleted},
	}

	tests := []struct {
		current  Stage
		expected Stage
	}{
		{StageInit, StagePOConversation},
		{StagePOConversation, StageAgentExecuting},
		{StageAgentExecuting, StageCompleted},
		{StageCompleted, StageFailed},     // last stage → failed
		{StageArchitectAnalysis, StageFailed}, // not in workflow → failed
	}

	for _, tt := range tests {
		got := wf.NextStageAfter(tt.current)
		if got != tt.expected {
			t.Errorf("NextStageAfter(%q) = %q, want %q", tt.current, got, tt.expected)
		}
	}
}

func TestHasStage(t *testing.T) {
	wf := &WorkflowTemplate{
		Stages: []Stage{StageInit, StagePOConversation, StageCompleted},
	}

	if !wf.HasStage(StageInit) {
		t.Error("should have init")
	}
	if !wf.HasStage(StagePOConversation) {
		t.Error("should have po_conversation")
	}
	if wf.HasStage(StageAgentExecuting) {
		t.Error("should not have agent_executing")
	}
}

// stageSetEqual compares two stage slices as sets (order-independent).
func stageSetEqual(a, b []Stage) bool {
	if len(a) != len(b) {
		return false
	}
	sa := make([]string, len(a))
	sb := make([]string, len(b))
	for i := range a {
		sa[i] = string(a[i])
		sb[i] = string(b[i])
	}
	sort.Strings(sa)
	sort.Strings(sb)
	for i := range sa {
		if sa[i] != sb[i] {
			return false
		}
	}
	return true
}
