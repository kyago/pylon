package orchestrator

import (
	"sort"
	"testing"

	"github.com/kyago/pylon/internal/workflow"
)

// TestFeatureWorkflow_MatchesHardcoded verifies that the feature workflow template
// produces exactly the same transition map as the hardcoded validTransitions.
// This is the key backward compatibility guarantee for Phase 1.
func TestFeatureWorkflow_MatchesHardcoded(t *testing.T) {
	wf, err := workflow.LoadWorkflow("feature", "")
	if err != nil {
		t.Fatalf("failed to load feature workflow: %v", err)
	}

	dynamic := wf.BuildTransitions()

	// Compare with hardcoded validTransitions (as sets, order-independent)
	for stage, hardcodedTargets := range validTransitions {
		dynamicTargets, ok := dynamic[stage]
		if !ok {
			t.Errorf("dynamic transitions missing stage %q", stage)
			continue
		}
		if !stageSetEqual(hardcodedTargets, dynamicTargets) {
			t.Errorf("stage %q: hardcoded=%v dynamic=%v", stage, hardcodedTargets, dynamicTargets)
		}
	}

	for stage := range dynamic {
		if _, ok := validTransitions[stage]; !ok {
			t.Errorf("dynamic transitions has extra stage %q not in hardcoded", stage)
		}
	}
}

// TestPipeline_DynamicTransitions verifies that SetTransitions overrides default behavior.
func TestPipeline_DynamicTransitions(t *testing.T) {
	p := NewPipeline("test-dyn", 2)

	// With default transitions: init → po_conversation is valid
	if !p.CanTransition(StagePOConversation) {
		t.Fatal("default: init → po_conversation should be valid")
	}

	// Set custom transitions: init can only go to agent_executing
	p.SetTransitions(map[Stage][]Stage{
		StageInit:           {StageAgentExecuting, StageFailed},
		StageAgentExecuting: {StageCompleted, StageFailed},
	})

	// Now init → po_conversation should be invalid
	if p.CanTransition(StagePOConversation) {
		t.Error("dynamic: init → po_conversation should be invalid")
	}

	// init → agent_executing should be valid
	if !p.CanTransition(StageAgentExecuting) {
		t.Error("dynamic: init → agent_executing should be valid")
	}

	// Transition should work
	if err := p.Transition(StageAgentExecuting); err != nil {
		t.Fatalf("dynamic transition failed: %v", err)
	}
	if p.CurrentStage != StageAgentExecuting {
		t.Errorf("expected stage agent_executing, got %q", p.CurrentStage)
	}
}

// TestPipeline_DynamicTransitions_Fallback verifies that without SetTransitions,
// the default validTransitions are used (backward compatibility).
func TestPipeline_DynamicTransitions_Fallback(t *testing.T) {
	p := NewPipeline("test-fallback", 2)
	// No SetTransitions called

	// Should use default transitions
	if !p.CanTransition(StagePOConversation) {
		t.Error("fallback: init → po_conversation should be valid")
	}
	if p.CanTransition(StageCompleted) {
		t.Error("fallback: init → completed should be invalid")
	}
}

// TestBugfixWorkflow_SkipsStages verifies that bugfix workflow transitions
// skip architect, PM, task_review stages.
func TestBugfixWorkflow_SkipsStages(t *testing.T) {
	wf, err := workflow.LoadWorkflow("bugfix", "")
	if err != nil {
		t.Fatalf("failed to load bugfix workflow: %v", err)
	}

	transitions := wf.BuildTransitions()

	// PO should go directly to agent_executing (skip architect, PM, task_review)
	poTargets := transitions[StagePOConversation]
	if !stageContains(poTargets, StageAgentExecuting) {
		t.Errorf("bugfix: po → agent_executing expected, got %v", poTargets)
	}
	if stageContains(poTargets, StageArchitectAnalysis) {
		t.Errorf("bugfix: po should NOT → architect_analysis, got %v", poTargets)
	}

	// Architect, PM, TaskReview should have NO transitions
	for _, skipped := range []Stage{StageArchitectAnalysis, StagePMTaskBreakdown, StageTaskReview} {
		if _, ok := transitions[skipped]; ok {
			t.Errorf("bugfix: skipped stage %q should have no transitions", skipped)
		}
	}

	// Verification should loopback to agent_executing
	verTargets := transitions[StageVerification]
	if !stageContains(verTargets, StageAgentExecuting) {
		t.Errorf("bugfix: verification should loopback to agent_executing, got %v", verTargets)
	}
}

// TestHotfixWorkflow_SkipsPO verifies that hotfix workflow skips PO conversation.
func TestHotfixWorkflow_SkipsPO(t *testing.T) {
	wf, err := workflow.LoadWorkflow("hotfix", "")
	if err != nil {
		t.Fatalf("failed to load hotfix workflow: %v", err)
	}

	// Hotfix should NOT include PO conversation
	if wf.HasStage(StagePOConversation) {
		t.Error("hotfix should not include po_conversation")
	}

	transitions := wf.BuildTransitions()

	// Init should go directly to agent_executing
	initTargets := transitions[StageInit]
	if !stageContains(initTargets, StageAgentExecuting) {
		t.Errorf("hotfix: init → agent_executing expected, got %v", initTargets)
	}
}

// TestWorkflow_PipelineIntegration verifies end-to-end: load workflow → build transitions
// → set on pipeline → transitions work correctly.
func TestWorkflow_PipelineIntegration(t *testing.T) {
	wf, err := workflow.LoadWorkflow("bugfix", "")
	if err != nil {
		t.Fatal(err)
	}

	p := NewPipeline("test-bugfix-integration", 2)
	p.WorkflowName = "bugfix"
	p.SetTransitions(wf.BuildTransitions())

	// init → po_conversation (bugfix starts with PO)
	if err := p.Transition(StagePOConversation); err != nil {
		t.Fatalf("init → po_conversation failed: %v", err)
	}

	// po_conversation → agent_executing (skip architect/PM/task_review)
	if err := p.Transition(StageAgentExecuting); err != nil {
		t.Fatalf("po → agent_executing failed: %v", err)
	}

	// agent_executing → verification
	if err := p.Transition(StageVerification); err != nil {
		t.Fatalf("agent → verification failed: %v", err)
	}

	// verification → pr_creation
	if err := p.Transition(StagePRCreation); err != nil {
		t.Fatalf("verification → pr_creation failed: %v", err)
	}

	// pr_creation → completed
	if err := p.Transition(StageCompleted); err != nil {
		t.Fatalf("pr_creation → completed failed: %v", err)
	}

	if !p.IsTerminal() {
		t.Error("pipeline should be terminal after completing bugfix workflow")
	}
}

// TestWorkflow_Recovery verifies that workflow name can be used to rebuild
// transitions after pipeline recovery from serialized state.
func TestWorkflow_Recovery(t *testing.T) {
	// Create a pipeline with bugfix workflow
	wf, err := workflow.LoadWorkflow("bugfix", "")
	if err != nil {
		t.Fatal(err)
	}

	p := NewPipeline("test-recovery", 2)
	p.WorkflowName = "bugfix"
	p.SetTransitions(wf.BuildTransitions())
	p.Transition(StagePOConversation)
	p.Transition(StageAgentExecuting)

	// Simulate crash: serialize and deserialize
	data, err := p.Snapshot()
	if err != nil {
		t.Fatal(err)
	}

	recovered, err := LoadPipeline(data)
	if err != nil {
		t.Fatal(err)
	}

	// After recovery, transitions are nil (not serialized)
	if recovered.CanTransition(StageVerification) {
		// This uses the default validTransitions, which also allows this transition
		// So this test verifies that the WorkflowName is preserved for re-applying
	}

	// Re-apply workflow from WorkflowName
	if recovered.WorkflowName != "bugfix" {
		t.Fatalf("expected workflow name 'bugfix', got %q", recovered.WorkflowName)
	}

	wf2, err := workflow.LoadWorkflow(recovered.WorkflowName, "")
	if err != nil {
		t.Fatal(err)
	}
	recovered.SetTransitions(wf2.BuildTransitions())

	// Now transitions should work per bugfix workflow
	if err := recovered.Transition(StageVerification); err != nil {
		t.Fatalf("recovered: agent_executing → verification failed: %v", err)
	}

	// Architect should NOT be reachable
	if recovered.CanTransition(StageArchitectAnalysis) {
		t.Error("recovered bugfix should not allow transition to architect_analysis")
	}
}

// TestWorkflow_NextStageAfter_Integration verifies NextStageAfter with various workflows.
func TestWorkflow_NextStageAfter_Integration(t *testing.T) {
	tests := []struct {
		workflow string
		from     Stage
		expected Stage
	}{
		{"feature", StageInit, StagePOConversation},
		{"feature", StagePOConversation, StageArchitectAnalysis},
		{"feature", StageWikiUpdate, StageCompleted},
		{"bugfix", StageInit, StagePOConversation},
		{"bugfix", StagePOConversation, StageAgentExecuting},
		{"bugfix", StagePRCreation, StageCompleted},
		{"hotfix", StageInit, StageAgentExecuting},
		{"docs", StageAgentExecuting, StageWikiUpdate},
		{"refactor", StageArchitectAnalysis, StageAgentExecuting},
	}

	for _, tt := range tests {
		wf, err := workflow.LoadWorkflow(tt.workflow, "")
		if err != nil {
			t.Fatalf("failed to load %s: %v", tt.workflow, err)
		}
		got := wf.NextStageAfter(tt.from)
		if got != tt.expected {
			t.Errorf("%s: NextStageAfter(%s) = %s, want %s", tt.workflow, tt.from, got, tt.expected)
		}
	}
}

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

func stageContains(stages []Stage, target Stage) bool {
	for _, s := range stages {
		if s == target {
			return true
		}
	}
	return false
}
