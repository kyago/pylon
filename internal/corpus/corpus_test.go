package corpus

import (
	"context"
	"testing"
	"time"
)

type fixtureExecutor struct {
	provider  string
	narrative string
}

func (executor fixtureExecutor) Execute(_ context.Context, fixture Fixture, _ string) (Outcome, error) {
	return Outcome{
		SchemaVersion: SchemaVersion, FixtureID: fixture.ID, Provider: executor.provider,
		Verdict: fixture.Expected.Verdict, RunStatus: fixture.Expected.RunStatus,
		TaskStatuses:     fixture.Expected.TaskStatuses,
		Artifacts:        append([]string(nil), fixture.Expected.RequiredArtifacts...),
		Events:           append([]string(nil), fixture.Expected.RequiredEvents...),
		CleanupStatus:    fixture.Expected.CleanupStatus,
		RuntimePreserved: fixture.Expected.RuntimePreserved,
		Narrative:        executor.narrative,
	}, nil
}

func TestEmbeddedCorpusCoversCoreContracts(t *testing.T) {
	fixtures, err := LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	if len(fixtures) < 10 || len(fixtures) > 20 {
		t.Fatalf("fixture count = %d, want 10-20", len(fixtures))
	}
	categories := make(map[string]bool)
	for _, fixture := range fixtures {
		categories[fixture.Category] = true
	}
	for _, required := range []string{"multi_repo", "resume", "state", "verification", "permissions", "cleanup", "failure_history"} {
		if !categories[required] {
			t.Fatalf("missing corpus category %q", required)
		}
	}
}

func TestProviderNarrativeDoesNotAffectMechanicalVerdict(t *testing.T) {
	fixtures, err := LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 10, 5, 0, 0, 0, time.UTC)
	first, err := Run(context.Background(), RunOptions{
		Fixtures: fixtures, Executor: fixtureExecutor{provider: "provider-a", narrative: "verbose explanation"},
		Timeout: time.Second, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Run(context.Background(), RunOptions{
		Fixtures: fixtures, Executor: fixtureExecutor{provider: "provider-b", narrative: "different wording"},
		Timeout: time.Second, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if !first.Passed || !second.Passed || len(first.Cases) != len(second.Cases) {
		t.Fatalf("provider-neutral reports failed: first=%+v second=%+v", first, second)
	}
	if err := ValidateReport(first, fixtures); err != nil {
		t.Fatalf("valid corpus report rejected: %v", err)
	}
	for index := range first.Cases {
		if first.Cases[index].Passed != second.Cases[index].Passed || len(first.Cases[index].Mismatches) != 0 || len(second.Cases[index].Mismatches) != 0 {
			t.Fatalf("provider text changed result at %d", index)
		}
	}
}

func TestValidateReportRejectsSubsetWithPassedFlag(t *testing.T) {
	fixtures, err := LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	report := Report{
		SchemaVersion: SchemaVersion, FixtureSetDigest: FixtureSetDigest(fixtures), Passed: true,
		Cases: []CaseResult{{FixtureID: fixtures[0].ID, Category: fixtures[0].Category, Passed: true}},
	}
	if err := ValidateReport(report, fixtures); err == nil {
		t.Fatal("subset report was accepted")
	}
}

func TestCompareDetectsForbiddenMutation(t *testing.T) {
	fixture := Fixture{
		SchemaVersion: SchemaVersion, ID: "permission-test", Category: "permissions", Description: "reviewer cannot edit",
		Input:    []byte(`{}`),
		Expected: ExpectedOutcome{Verdict: "fail", RunStatus: "failed", ForbiddenMutations: []string{"implementation/file.go"}},
	}
	outcome := Outcome{SchemaVersion: SchemaVersion, FixtureID: fixture.ID, Verdict: "fail", RunStatus: "failed", Mutations: []string{"implementation/file.go"}}
	if mismatches := Compare(fixture, outcome); len(mismatches) != 1 {
		t.Fatalf("mutation mismatch = %v", mismatches)
	}
}
