package corpus

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Executor interface {
	Execute(context.Context, Fixture, string) (Outcome, error)
}

type CaseResult struct {
	FixtureID  string   `json:"fixture_id"`
	Category   string   `json:"category"`
	Passed     bool     `json:"passed"`
	Mismatches []string `json:"mismatches,omitempty"`
	Outcome    Outcome  `json:"outcome"`
}

type Report struct {
	SchemaVersion    int          `json:"schema_version"`
	FixtureSetDigest string       `json:"fixture_set_digest"`
	Passed           bool         `json:"passed"`
	StartedAt        time.Time    `json:"started_at"`
	CompletedAt      time.Time    `json:"completed_at"`
	Cases            []CaseResult `json:"cases"`
}

type RunOptions struct {
	Fixtures []Fixture
	Executor Executor
	TempRoot string
	Timeout  time.Duration
	Now      func() time.Time
}

func Run(ctx context.Context, options RunOptions) (Report, error) {
	if len(options.Fixtures) == 0 {
		return Report{}, fmt.Errorf("no corpus fixtures supplied")
	}
	if options.Executor == nil {
		return Report{}, fmt.Errorf("corpus executor is required")
	}
	if options.Timeout <= 0 {
		options.Timeout = 5 * time.Minute
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	report := Report{
		SchemaVersion: SchemaVersion, FixtureSetDigest: FixtureSetDigest(options.Fixtures),
		Passed: true, StartedAt: now().UTC(),
	}
	for _, fixture := range options.Fixtures {
		workDir, err := os.MkdirTemp(options.TempRoot, "pylon-corpus-"+fixture.ID+"-")
		if err != nil {
			return Report{}, err
		}
		caseContext, cancel := context.WithTimeout(ctx, options.Timeout)
		outcome, executeErr := options.Executor.Execute(caseContext, fixture, workDir)
		cancel()
		_ = os.RemoveAll(workDir)
		mismatches := make([]string, 0)
		if executeErr != nil {
			mismatches = append(mismatches, "executor error: "+executeErr.Error())
		} else {
			mismatches = Compare(fixture, outcome)
		}
		passed := len(mismatches) == 0
		if !passed {
			report.Passed = false
		}
		report.Cases = append(report.Cases, CaseResult{
			FixtureID: fixture.ID, Category: fixture.Category, Passed: passed,
			Mismatches: mismatches, Outcome: outcome,
		})
	}
	report.CompletedAt = now().UTC()
	return report, nil
}

// FixtureSetDigest binds a report to the complete normalized fixture corpus.
func FixtureSetDigest(fixtures []Fixture) string {
	normalized := append([]Fixture(nil), fixtures...)
	sort.Slice(normalized, func(i, j int) bool { return normalized[i].ID < normalized[j].ID })
	data, _ := json.Marshal(normalized)
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// ValidateReport mechanically replays each reported outcome against the exact
// expected fixture set. Subset, duplicate, missing, or internally inconsistent
// reports are rejected even when their top-level passed flag is true.
func ValidateReport(report Report, fixtures []Fixture) error {
	if report.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported report schema version %d", report.SchemaVersion)
	}
	if !report.Passed {
		return fmt.Errorf("corpus report did not pass")
	}
	if report.FixtureSetDigest != FixtureSetDigest(fixtures) {
		return fmt.Errorf("fixture set digest mismatch")
	}
	if len(report.Cases) != len(fixtures) {
		return fmt.Errorf("case count=%d want %d", len(report.Cases), len(fixtures))
	}
	expected := make(map[string]Fixture, len(fixtures))
	for _, fixture := range fixtures {
		expected[fixture.ID] = fixture
	}
	seen := make(map[string]bool, len(report.Cases))
	for _, result := range report.Cases {
		fixture, ok := expected[result.FixtureID]
		if !ok {
			return fmt.Errorf("unexpected fixture %q", result.FixtureID)
		}
		if seen[result.FixtureID] {
			return fmt.Errorf("duplicate fixture %q", result.FixtureID)
		}
		seen[result.FixtureID] = true
		if result.Category != fixture.Category {
			return fmt.Errorf("fixture %s category=%q want %q", fixture.ID, result.Category, fixture.Category)
		}
		if !result.Passed || len(result.Mismatches) != 0 {
			return fmt.Errorf("fixture %s did not pass", fixture.ID)
		}
		if mismatches := Compare(fixture, result.Outcome); len(mismatches) != 0 {
			return fmt.Errorf("fixture %s outcome mismatch: %s", fixture.ID, strings.Join(mismatches, "; "))
		}
	}
	return nil
}

type CommandExecutor struct {
	Executable string
	Args       []string
	Env        []string
}

func (executor CommandExecutor) Execute(ctx context.Context, fixture Fixture, workDir string) (Outcome, error) {
	var outcome Outcome
	if executor.Executable == "" {
		return outcome, fmt.Errorf("driver executable is required")
	}
	input, err := json.Marshal(fixture)
	if err != nil {
		return outcome, err
	}
	command := exec.CommandContext(ctx, executor.Executable, executor.Args...)
	command.Dir = workDir
	command.Env = append(os.Environ(), executor.Env...)
	command.Env = append(command.Env,
		"PYLON_CORPUS_FIXTURE_ID="+fixture.ID,
		"PYLON_CORPUS_WORKDIR="+workDir,
	)
	command.Stdin = bytes.NewReader(input)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return outcome, fmt.Errorf("driver failed: %w: %s", err, stderr.String())
	}
	decoder := json.NewDecoder(&stdout)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&outcome); err != nil {
		return outcome, fmt.Errorf("driver outcome parse failed: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return outcome, fmt.Errorf("driver returned multiple JSON values")
		}
		return outcome, fmt.Errorf("driver returned trailing invalid JSON: %w", err)
	}
	return outcome, nil
}

type RecordedExecutor struct {
	Dir string
}

func (executor RecordedExecutor) Execute(_ context.Context, fixture Fixture, _ string) (Outcome, error) {
	var outcome Outcome
	path := filepath.Join(executor.Dir, fixture.ID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return outcome, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&outcome); err != nil {
		return outcome, fmt.Errorf("%s outcome parse failed: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return outcome, fmt.Errorf("%s contains multiple JSON values", path)
		}
		return outcome, fmt.Errorf("%s trailing JSON parse failed: %w", path, err)
	}
	return outcome, nil
}
