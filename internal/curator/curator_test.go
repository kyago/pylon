package curator

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kyago/pylon/internal/corpus"
	"github.com/kyago/pylon/internal/history"
	"github.com/kyago/pylon/internal/layout"
)

func TestCreateCandidateFromFinalizedCheckpointDoesNotModifyActiveKnowledge(t *testing.T) {
	root, ref := setupCheckpoint(t, history.PhaseCompleted, true, "pass", false)
	activePath := filepath.Join(root, ".pylon", "memory", "app", "learning", "active.md")
	writeCuratorFile(t, activePath, "active knowledge\n")
	now := time.Date(2026, 8, 10, 6, 0, 0, 0, time.UTC)
	candidate, err := Create(CreateOptions{
		Root: root, CheckpointRef: ref, Now: func() time.Time { return now },
		Proposal: Proposal{
			Type: "memory", Title: "Preserve failed verification evidence",
			Summary:            "Record evidence references before cleanup.",
			Rationale:          "The finalized run demonstrates that cleanup must remain gated.",
			TargetFiles:        []string{".pylon/memory/app/learning/cleanup.md"},
			EvidenceRefs:       []string{"status-summary.json"},
			RegressionFixtures: []string{"cleanup-terminal-checkpoint-gate"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Status.Status != "pending_review" || candidate.Status.SourceCheckpoint != ref {
		t.Fatalf("unexpected candidate: %+v", candidate)
	}
	for _, name := range []string{"proposal.md", "evidence.json", "source-runs.json", "target-files.json", "status.json"} {
		if _, err := os.Stat(filepath.Join(candidate.Path, name)); err != nil {
			t.Fatalf("candidate file %s missing: %v", name, err)
		}
	}
	data, err := os.ReadFile(activePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "active knowledge\n" {
		t.Fatalf("active knowledge changed without approval: %s", data)
	}
}

func TestCreateRejectsUnfinishedOrUnconfirmedRuns(t *testing.T) {
	root, ref := setupCheckpoint(t, history.PhaseExecuted, true, "pass", false)
	_, err := Create(CreateOptions{Root: root, CheckpointRef: ref, Proposal: validProposal()})
	if !errors.Is(err, ErrNotFinalized) {
		t.Fatalf("executed checkpoint error = %v", err)
	}

	root, ref = setupCheckpoint(t, history.PhaseCompleted, true, "incomplete", false)
	_, err = Create(CreateOptions{Root: root, CheckpointRef: ref, Proposal: validProposal()})
	if !errors.Is(err, ErrNotFinalized) {
		t.Fatalf("incomplete evaluator error = %v", err)
	}

	root, ref = setupCheckpoint(t, history.PhaseFailed, false, "fail", false)
	_, err = Create(CreateOptions{Root: root, CheckpointRef: ref, Proposal: validProposal()})
	if !errors.Is(err, ErrNotFinalized) {
		t.Fatalf("failed run without failure record error = %v", err)
	}
}

func TestReviewAndRegressionGateOnlyChangeCandidateStatus(t *testing.T) {
	root, ref := setupCheckpoint(t, history.PhaseFailed, false, "fail", true)
	candidate, err := Create(CreateOptions{Root: root, CheckpointRef: ref, Proposal: validProposal()})
	if err != nil {
		t.Fatal(err)
	}
	approved, err := Review(root, candidate.ID, "approve", "evidence is reusable", nil)
	if err != nil {
		t.Fatal(err)
	}
	if approved.Status != "approved" || len(approved.Decisions) != 1 {
		t.Fatalf("unexpected approval: %+v", approved)
	}
	reportPath := filepath.Join(root, "corpus-report.json")
	writeCuratorJSON(t, reportPath, passingCorpusReport(t))
	gated, err := Gate(root, candidate.ID, reportPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if gated.Status != "regression_passed" || gated.RegressionGate == nil || gated.RegressionGate.CaseCount < 10 {
		t.Fatalf("unexpected gate status: %+v", gated)
	}
	if _, err := os.Stat(filepath.Join(layout.LearningCandidatesDir(root), candidate.ID)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".pylon", "memory", "app", "learning", "new.md")); !os.IsNotExist(err) {
		t.Fatalf("candidate workflow modified active target: %v", err)
	}
}

func TestRegressionGateRejectsForgedSubsetReport(t *testing.T) {
	root, ref := setupCheckpoint(t, history.PhaseFailed, false, "fail", true)
	candidate, err := Create(CreateOptions{Root: root, CheckpointRef: ref, Proposal: validProposal()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Review(root, candidate.ID, "approve", "approved", nil); err != nil {
		t.Fatal(err)
	}
	reportPath := filepath.Join(root, "forged-report.json")
	writeCuratorJSON(t, reportPath, corpus.Report{
		SchemaVersion: corpus.SchemaVersion, Passed: true,
		Cases: []corpus.CaseResult{{FixtureID: "failure-history-rejected-hypotheses", Passed: true}},
	})
	if _, err := Gate(root, candidate.ID, reportPath, nil); err == nil {
		t.Fatal("forged subset report passed regression gate")
	}
}

func TestReviewRejectsTamperedCandidate(t *testing.T) {
	root, ref := setupCheckpoint(t, history.PhaseCompleted, true, "pass", false)
	candidate, err := Create(CreateOptions{Root: root, CheckpointRef: ref, Proposal: validProposal()})
	if err != nil {
		t.Fatal(err)
	}
	writeCuratorFile(t, filepath.Join(candidate.Path, "proposal.md"), "tampered proposal\n")
	if _, err := Review(root, candidate.ID, "approve", "looks good", nil); err == nil {
		t.Fatal("tampered candidate was approved")
	}
}

func setupCheckpoint(t *testing.T, phase history.Phase, verificationOK bool, evaluatorStatus string, includeFailure bool) (string, string) {
	t.Helper()
	root := t.TempDir()
	pipelineID := "run-1"
	runtimeDir := filepath.Join(root, ".pylon", "runtime", pipelineID)
	writeCuratorFile(t, filepath.Join(runtimeDir, "requirement.md"), "# requirement\n")
	writeCuratorFile(t, filepath.Join(runtimeDir, "tasks.json"), `{"tasks":[{"id":"T001","repo":"app"}]}`)
	writeCuratorJSON(t, filepath.Join(runtimeDir, "status.json"), map[string]any{"pipeline_id": pipelineID, "status": string(phase), "stage": string(phase)})
	writeCuratorJSON(t, filepath.Join(runtimeDir, "criteria.json"), map[string]any{
		"schema_version": 1, "run_id": pipelineID, "repo_id": "app", "base_revision": "abc",
		"acceptance_criteria": []map[string]any{{"id": "AC-1", "description": "works"}},
		"verification":        []map[string]any{{"name": "test", "kind": "deterministic", "command": "true"}},
		"digest":              "sha256:criteria",
	})
	writeCuratorJSON(t, filepath.Join(runtimeDir, "verification.json"), map[string]any{
		"ok": verificationOK, "criteria_digest": "sha256:criteria", "checks": []map[string]any{{"name": "test", "ok": verificationOK}},
	})
	writeCuratorJSON(t, filepath.Join(runtimeDir, "evaluator-result.json"), map[string]any{
		"schema_version": 1, "request_digest": "sha256:request", "status": evaluatorStatus,
		"summary": "reviewed", "criteria": []map[string]any{{"id": "AC-1", "status": evaluatorStatus, "evidence": "change.diff"}},
		"evaluator": "verifier",
	})
	writeCuratorJSON(t, filepath.Join(runtimeDir, "task-report.json"), map[string]any{
		"schema_version": 1, "run_id": pipelineID, "task_id": "T001", "attempt": 1,
		"status":  map[bool]string{true: "succeeded", false: "failed"}[verificationOK],
		"summary": "task finalized", "digest": "sha256:task-report",
	})
	if includeFailure {
		writeCuratorJSON(t, filepath.Join(runtimeDir, "failure-record.json"), map[string]any{
			"schema_version": 1, "run_id": pipelineID, "task_id": "T001", "attempt": 1,
			"phase": "verification", "terminal_cause": "test_failure", "abandoned_reason": "terminal failure",
			"recorded_at": "2026-08-10T06:00:00Z", "digest": "sha256:failure",
		})
	}
	manager := history.NewManager(root)
	if _, err := manager.Checkpoint(pipelineID, phase); err != nil {
		t.Fatal(err)
	}
	return root, pipelineID + "/" + string(phase)
}

func validProposal() Proposal {
	return Proposal{
		Type: "pitfall", Title: "Keep failure checkpoints",
		Summary: "Preserve failure evidence.", Rationale: "The run demonstrates the cleanup hazard.",
		TargetFiles: []string{".pylon/memory/app/learning/new.md"},
	}
}

type curatorFixtureExecutor struct{}

func (curatorFixtureExecutor) Execute(_ context.Context, fixture corpus.Fixture, _ string) (corpus.Outcome, error) {
	return corpus.Outcome{
		SchemaVersion: corpus.SchemaVersion, FixtureID: fixture.ID,
		Verdict: fixture.Expected.Verdict, RunStatus: fixture.Expected.RunStatus,
		TaskStatuses:     fixture.Expected.TaskStatuses,
		Artifacts:        append([]string(nil), fixture.Expected.RequiredArtifacts...),
		Events:           append([]string(nil), fixture.Expected.RequiredEvents...),
		CleanupStatus:    fixture.Expected.CleanupStatus,
		RuntimePreserved: fixture.Expected.RuntimePreserved,
	}, nil
}

func passingCorpusReport(t *testing.T) corpus.Report {
	t.Helper()
	fixtures, err := corpus.LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	report, err := corpus.Run(context.Background(), corpus.RunOptions{
		Fixtures: fixtures, Executor: curatorFixtureExecutor{}, Timeout: time.Second,
		Now: func() time.Time { return time.Date(2026, 8, 10, 6, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func writeCuratorFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func writeCuratorJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	writeCuratorFile(t, path, string(data))
}
