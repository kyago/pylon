package trajectory

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRecordTaskReportAndFailure(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 8, 10, 3, 0, 0, 0, time.UTC)
	reportPath := filepath.Join(root, "tasks", "T001", "attempts", "002", "task-report.json")
	report, err := RecordTaskReport(TaskReport{
		RunID: "run-1", TaskID: "T001", Attempt: 2, Status: "failed", Summary: "verification failed",
		Provider:           &ProviderSummary{Name: "fake", Capabilities: []string{"run_shell", "read_files", "read_files"}},
		EvidenceRefs:       []string{"tasks/T001/attempts/002/stderr.log"},
		HypothesesRejected: []RejectedHypothesis{{Hypothesis: "wrong config path", Probe: "resolve path", Result: "path is correct"}},
	}, RecordOptions{OutputPath: reportPath, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	if report.Digest == "" || report.CompletedAt != now || len(report.Provider.Capabilities) != 2 {
		t.Fatalf("unexpected task report: %+v", report)
	}
	if _, err := LoadTaskReport(reportPath); err != nil {
		t.Fatal(err)
	}

	failurePath := filepath.Join(root, "failure-record.json")
	failure, err := RecordFailure(FailureRecord{
		RunID: "run-1", TaskID: "T001", Attempt: 2, Phase: "verification", TerminalCause: "test_failure",
		EvidenceRefs: report.EvidenceRefs, HypothesesRejected: report.HypothesesRejected,
		RemainingUnknowns: []string{"intermittent race"}, AbandonedReason: "maximum attempts exhausted",
	}, RecordOptions{OutputPath: failurePath, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	if failure.RecordedAt != now || failure.Digest == "" {
		t.Fatalf("unexpected failure record: %+v", failure)
	}
	if _, err := LoadFailure(failurePath); err != nil {
		t.Fatal(err)
	}
}

func TestTrajectoryRejectsUnsafeEvidenceAndTampering(t *testing.T) {
	path := filepath.Join(t.TempDir(), "failure.json")
	_, err := RecordFailure(FailureRecord{
		RunID: "run-1", TaskID: "T001", Attempt: 1, Phase: "execution", TerminalCause: "worker_failed",
		EvidenceRefs: []string{"../conversation.md"}, AbandonedReason: "terminal worker failure",
	}, RecordOptions{OutputPath: path})
	if !errors.Is(err, ErrInvalidArtifact) {
		t.Fatalf("unsafe evidence error = %v", err)
	}

	record, err := RecordFailure(FailureRecord{
		RunID: "run-1", TaskID: "T001", Attempt: 1, Phase: "execution", TerminalCause: "worker_failed",
		EvidenceRefs: []string{"tasks/T001/stderr.log"}, AbandonedReason: "terminal worker failure",
	}, RecordOptions{OutputPath: path})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tampered := append([]byte(nil), data...)
	for index := range tampered {
		if tampered[index] == 'e' {
			tampered[index] = 'x'
			break
		}
	}
	if err := os.WriteFile(path, tampered, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFailure(path); err == nil {
		t.Fatalf("tampered failure accepted: %s", record.Digest)
	}
}

func TestTrajectoryCreateOnceIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.json")
	now := time.Date(2026, 8, 10, 3, 0, 0, 0, time.UTC)
	input := TaskReport{RunID: "run-1", TaskID: "T001", Attempt: 1, Status: "succeeded", Summary: "done"}
	first, err := RecordTaskReport(input, RecordOptions{OutputPath: path, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	second, err := RecordTaskReport(input, RecordOptions{OutputPath: path, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest != second.Digest {
		t.Fatalf("idempotent digest mismatch: %s != %s", first.Digest, second.Digest)
	}
	input.Summary = "different"
	if _, err := RecordTaskReport(input, RecordOptions{OutputPath: path, Now: func() time.Time { return now }}); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("conflicting report error = %v", err)
	}
}
