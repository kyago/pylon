package evaluator

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kyago/pylon/internal/criteria"
	"github.com/kyago/pylon/internal/provider"
)

func TestPrepareCreatesReadOnlyEvidenceOnlyBundle(t *testing.T) {
	fixture := newFixture(t)
	request, err := Prepare(fixture.options)
	if err != nil {
		t.Fatal(err)
	}
	if request.Policy.AccessMode != "read_only" || request.Policy.InputPolicy != "isolated_evidence" {
		t.Fatalf("unsafe policy: %+v", request.Policy)
	}
	if len(request.Inputs) != 5 || len(request.CriteriaIDs) != 1 || request.CriteriaIDs[0] != "AC-1" {
		t.Fatalf("unexpected request inputs: %+v", request)
	}
	for _, forbidden := range []string{"memory", "conversation", "transcript"} {
		for _, input := range request.Inputs {
			if input.Name == forbidden {
				t.Fatalf("forbidden input included: %+v", input)
			}
		}
	}
	loaded, err := Load(fixture.options.OutputDir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Digest != request.Digest {
		t.Fatalf("loaded digest = %q, want %q", loaded.Digest, request.Digest)
	}
	for _, input := range request.Inputs {
		info, err := os.Stat(filepath.Join(fixture.options.OutputDir, input.Path))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0222 != 0 {
			t.Fatalf("input is writable: %s %o", input.Name, info.Mode().Perm())
		}
	}
}

func TestPrepareRejectsUnsafeContextAndFailedGate(t *testing.T) {
	fixture := newFixture(t)
	fixture.options.OutputDir = filepath.Join(fixture.options.ImplementationRoot, "evaluator")
	if _, err := Prepare(fixture.options); !errors.Is(err, ErrUnsafeOutputPath) {
		t.Fatalf("unsafe output error = %v", err)
	}

	fixture = newFixture(t)
	memoryDir := filepath.Join(fixture.options.ImplementationRoot, ".pylon", "memory")
	if err := os.MkdirAll(memoryDir, 0755); err != nil {
		t.Fatal(err)
	}
	memoryPath := filepath.Join(memoryDir, "secret.md")
	if err := os.WriteFile(memoryPath, []byte("private memory"), 0644); err != nil {
		t.Fatal(err)
	}
	fixture.options.RequirementPath = memoryPath
	if _, err := Prepare(fixture.options); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("forbidden context error = %v", err)
	}

	fixture = newFixture(t)
	verificationData, err := os.ReadFile(fixture.options.VerificationPath)
	if err != nil {
		t.Fatal(err)
	}
	var verification map[string]any
	if err := json.Unmarshal(verificationData, &verification); err != nil {
		t.Fatal(err)
	}
	verification["ok"] = false
	writeJSON(t, fixture.options.VerificationPath, verification)
	if _, err := Prepare(fixture.options); !errors.Is(err, ErrInsufficientGate) {
		t.Fatalf("failed deterministic gate error = %v", err)
	}
}

func TestLoadDetectsBundleTampering(t *testing.T) {
	fixture := newFixture(t)
	if _, err := Prepare(fixture.options); err != nil {
		t.Fatal(err)
	}
	diffPath := filepath.Join(fixture.options.OutputDir, "change.diff")
	if err := os.Chmod(diffPath, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(diffPath, []byte("tampered"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(fixture.options.OutputDir); !errors.Is(err, ErrIntegrityFailure) {
		t.Fatalf("tampered bundle error = %v", err)
	}
}

func TestLoadBoundDetectsManifestTampering(t *testing.T) {
	fixture := newFixture(t)
	if _, err := Prepare(fixture.options); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(fixture.options.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest["evaluator_request"].(map[string]any)["digest"] = "sha256:tampered"
	writeJSON(t, fixture.options.ManifestPath, manifest)
	if _, err := LoadBound(fixture.options.OutputDir, fixture.options.ManifestPath); !errors.Is(err, ErrIntegrityFailure) {
		t.Fatalf("manifest tampering error = %v", err)
	}
}

func TestPolicyAndVerdictValidation(t *testing.T) {
	policy := DefaultPolicy()
	if err := policy.ValidateProvider(provider.CapabilitySet{ReadFiles: true, StructuredOutput: true}); !errors.Is(err, ErrPolicyUnsupported) {
		t.Fatalf("provider without restrictions error = %v", err)
	}
	if err := policy.ValidateProvider(provider.CapabilitySet{ReadFiles: true, StructuredOutput: true, ToolRestrictions: true, EditFiles: true}); err != nil {
		t.Fatalf("restrictable provider rejected: %v", err)
	}
	request := Request{
		Digest:      "request-digest",
		CriteriaIDs: []string{"AC-1", "AC-2"},
		Inputs:      []InputFile{{Name: "change.diff", Path: "change.diff"}},
	}
	verdict := Verdict{
		SchemaVersion: SchemaVersion,
		RequestDigest: request.Digest,
		Status:        VerdictPass,
		Summary:       "all criteria are covered",
		Evaluator:     "fake",
		Criteria: []CriterionVerdict{
			{ID: "AC-1", Status: "verified", Evidence: "criteria.json and change.diff"},
			{ID: "AC-2", Status: "partial", Evidence: "missing edge case"},
		},
		EvidenceRefs: []string{"change.diff"},
	}
	if err := ValidateVerdict(request, verdict); err == nil {
		t.Fatal("pass verdict with partial criterion was accepted")
	}
	verdict.Status = VerdictFail
	if err := ValidateVerdict(request, verdict); err != nil {
		t.Fatalf("valid fail verdict rejected: %v", err)
	}
	verdict.EvidenceRefs = []string{"../memory/secret.md"}
	if err := ValidateVerdict(request, verdict); err == nil {
		t.Fatal("out-of-bundle evidence reference was accepted")
	}
}

func TestRecordValidatesAndWritesVerdict(t *testing.T) {
	fixture := newFixture(t)
	request, err := Prepare(fixture.options)
	if err != nil {
		t.Fatal(err)
	}
	inputPath := filepath.Join(filepath.Dir(fixture.options.OutputDir), "raw-verdict.json")
	outputPath := filepath.Join(filepath.Dir(fixture.options.OutputDir), "evaluator-result.json")
	writeJSON(t, inputPath, Verdict{
		SchemaVersion: SchemaVersion,
		RequestDigest: request.Digest,
		Status:        VerdictPass,
		Summary:       "criterion is covered",
		Evaluator:     "fake-evaluator",
		Criteria:      []CriterionVerdict{{ID: "AC-1", Status: "verified", Evidence: "change.diff"}},
	})
	recorded, err := Record(RecordOptions{
		BundleDir:    fixture.options.OutputDir,
		ManifestPath: fixture.options.ManifestPath,
		InputPath:    inputPath,
		OutputPath:   outputPath,
		Now:          func() time.Time { return time.Date(2026, 8, 10, 2, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if recorded.RecordedAt.IsZero() {
		t.Fatal("control plane did not record verdict timestamp")
	}
	if _, err := os.Stat(outputPath); err != nil {
		t.Fatal(err)
	}
}

type fixture struct {
	options PrepareOptions
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	root := t.TempDir()
	implementationRoot := filepath.Join(root, "repo")
	runtimeDir := filepath.Join(root, "runtime")
	if err := os.MkdirAll(filepath.Join(implementationRoot, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(implementationRoot, ".pylon", "verify.yml")
	if err := os.WriteFile(configPath, []byte("build:\n  command: \"true\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	criteriaPath := filepath.Join(runtimeDir, "criteria.json")
	manifestPath := filepath.Join(runtimeDir, "status.json")
	if err := os.WriteFile(manifestPath, []byte(`{"root_pipeline_id":"run","repo_id":"repo","base_revision":"base"}`), 0644); err != nil {
		t.Fatal(err)
	}
	snapshot, err := criteria.Create(criteria.CreateOptions{
		RunID: "run", RepoID: "repo", BaseRevision: "base", WorkDir: implementationRoot,
		ConfigPath:         configPath,
		OutputPath:         criteriaPath,
		ManifestPath:       manifestPath,
		AcceptanceCriteria: []provider.Criterion{{ID: "AC-1", Description: "feature works"}},
		Now:                func() time.Time { return time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	requirementPath := filepath.Join(implementationRoot, "requirement.md")
	diffPath := filepath.Join(implementationRoot, "change.diff")
	taskReportPath := filepath.Join(runtimeDir, "task-report.json")
	verificationPath := filepath.Join(runtimeDir, "verification.json")
	for path, content := range map[string]string{
		requirementPath:  "implement the feature",
		diffPath:         "diff --git a/a b/a",
		taskReportPath:   `{"summary":"implemented"}`,
		verificationPath: `{"ok":true,"criteria_digest":"` + snapshot.Digest + `"}`,
	} {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	result := fixture{options: PrepareOptions{
		TaskID:             "task",
		ImplementationRoot: implementationRoot,
		RequirementPath:    requirementPath,
		CriteriaPath:       criteriaPath,
		VerificationPath:   verificationPath,
		DiffPath:           diffPath,
		TaskReportPath:     taskReportPath,
		OutputDir:          filepath.Join(runtimeDir, "evaluator-input"),
		ManifestPath:       manifestPath,
		Now:                func() time.Time { return time.Date(2026, 8, 10, 1, 0, 0, 0, time.UTC) },
	}}
	t.Cleanup(func() { _ = makeWritable(result.options.OutputDir) })
	return result
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
}
