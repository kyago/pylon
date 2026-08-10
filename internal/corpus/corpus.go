// Package corpus defines provider-neutral acceptance fixtures and result matching.
package corpus

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

const SchemaVersion = 1

var ErrInvalidFixture = errors.New("invalid acceptance corpus fixture")

//go:embed fixtures/*.json
var embeddedFixtures embed.FS

type ExpectedOutcome struct {
	Verdict            string            `json:"verdict"`
	RunStatus          string            `json:"run_status"`
	TaskStatuses       map[string]string `json:"task_statuses,omitempty"`
	RequiredArtifacts  []string          `json:"required_artifacts,omitempty"`
	ForbiddenMutations []string          `json:"forbidden_mutations,omitempty"`
	RequiredEvents     []string          `json:"required_events,omitempty"`
	CleanupStatus      string            `json:"cleanup_status,omitempty"`
	RuntimePreserved   bool              `json:"runtime_preserved,omitempty"`
}

type Fixture struct {
	SchemaVersion        int             `json:"schema_version"`
	ID                   string          `json:"id"`
	Category             string          `json:"category"`
	Description          string          `json:"description"`
	RequiredCapabilities []string        `json:"required_capabilities,omitempty"`
	Input                json.RawMessage `json:"input"`
	Expected             ExpectedOutcome `json:"expected"`
}

type Outcome struct {
	SchemaVersion    int               `json:"schema_version"`
	FixtureID        string            `json:"fixture_id"`
	Provider         string            `json:"provider,omitempty"`
	Verdict          string            `json:"verdict"`
	RunStatus        string            `json:"run_status"`
	TaskStatuses     map[string]string `json:"task_statuses,omitempty"`
	Artifacts        []string          `json:"artifacts,omitempty"`
	Mutations        []string          `json:"mutations,omitempty"`
	Events           []string          `json:"events,omitempty"`
	CleanupStatus    string            `json:"cleanup_status,omitempty"`
	RuntimePreserved bool              `json:"runtime_preserved,omitempty"`
	Narrative        string            `json:"narrative,omitempty"`
}

func LoadEmbedded() ([]Fixture, error) {
	return LoadFS(embeddedFixtures, "fixtures/*.json")
}

func LoadDir(dir string) ([]Fixture, error) {
	return LoadFS(osDirFS(dir), "*.json")
}

func LoadFS(source fs.FS, pattern string) ([]Fixture, error) {
	paths, err := fs.Glob(source, pattern)
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	fixtures := make([]Fixture, 0, len(paths))
	seen := make(map[string]bool)
	for _, path := range paths {
		data, err := fs.ReadFile(source, path)
		if err != nil {
			return nil, err
		}
		var fixture Fixture
		if err := json.Unmarshal(data, &fixture); err != nil {
			return nil, fmt.Errorf("%s JSON parse failed: %w", path, err)
		}
		if err := ValidateFixture(fixture); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if seen[fixture.ID] {
			return nil, fmt.Errorf("%w: duplicate fixture id %q", ErrInvalidFixture, fixture.ID)
		}
		seen[fixture.ID] = true
		fixtures = append(fixtures, fixture)
	}
	if len(fixtures) == 0 {
		return nil, fmt.Errorf("%w: no fixtures matched %q", ErrInvalidFixture, pattern)
	}
	return fixtures, nil
}

func ValidateFixture(fixture Fixture) error {
	if fixture.SchemaVersion != SchemaVersion {
		return fmt.Errorf("%w: unsupported schema version %d", ErrInvalidFixture, fixture.SchemaVersion)
	}
	if !safeID(fixture.ID) || strings.TrimSpace(fixture.Category) == "" || strings.TrimSpace(fixture.Description) == "" {
		return fmt.Errorf("%w: id, category, and description are required", ErrInvalidFixture)
	}
	if len(fixture.Input) == 0 || !json.Valid(fixture.Input) {
		return fmt.Errorf("%w: input must be valid JSON", ErrInvalidFixture)
	}
	if fixture.Expected.Verdict != "pass" && fixture.Expected.Verdict != "fail" {
		return fmt.Errorf("%w: expected verdict must be pass or fail", ErrInvalidFixture)
	}
	if strings.TrimSpace(fixture.Expected.RunStatus) == "" {
		return fmt.Errorf("%w: expected run status is required", ErrInvalidFixture)
	}
	return nil
}

func Compare(fixture Fixture, outcome Outcome) []string {
	mismatches := make([]string, 0)
	if outcome.SchemaVersion != SchemaVersion {
		mismatches = append(mismatches, fmt.Sprintf("schema_version=%d want %d", outcome.SchemaVersion, SchemaVersion))
	}
	if outcome.FixtureID != fixture.ID {
		mismatches = append(mismatches, fmt.Sprintf("fixture_id=%q want %q", outcome.FixtureID, fixture.ID))
	}
	if outcome.Verdict != fixture.Expected.Verdict {
		mismatches = append(mismatches, fmt.Sprintf("verdict=%q want %q", outcome.Verdict, fixture.Expected.Verdict))
	}
	if outcome.RunStatus != fixture.Expected.RunStatus {
		mismatches = append(mismatches, fmt.Sprintf("run_status=%q want %q", outcome.RunStatus, fixture.Expected.RunStatus))
	}
	for taskID, expected := range fixture.Expected.TaskStatuses {
		if actual := outcome.TaskStatuses[taskID]; actual != expected {
			mismatches = append(mismatches, fmt.Sprintf("task %s status=%q want %q", taskID, actual, expected))
		}
	}
	mismatches = append(mismatches, missingSet("artifact", fixture.Expected.RequiredArtifacts, outcome.Artifacts)...)
	mismatches = append(mismatches, presentForbidden("mutation", fixture.Expected.ForbiddenMutations, outcome.Mutations)...)
	mismatches = append(mismatches, missingSet("event", fixture.Expected.RequiredEvents, outcome.Events)...)
	if fixture.Expected.CleanupStatus != "" && outcome.CleanupStatus != fixture.Expected.CleanupStatus {
		mismatches = append(mismatches, fmt.Sprintf("cleanup_status=%q want %q", outcome.CleanupStatus, fixture.Expected.CleanupStatus))
	}
	if outcome.RuntimePreserved != fixture.Expected.RuntimePreserved {
		mismatches = append(mismatches, fmt.Sprintf("runtime_preserved=%t want %t", outcome.RuntimePreserved, fixture.Expected.RuntimePreserved))
	}
	sort.Strings(mismatches)
	return mismatches
}

func missingSet(kind string, required, actual []string) []string {
	present := make(map[string]bool, len(actual))
	for _, value := range actual {
		present[value] = true
	}
	missing := make([]string, 0)
	for _, value := range required {
		if !present[value] {
			missing = append(missing, fmt.Sprintf("missing %s %q", kind, value))
		}
	}
	return missing
}

func presentForbidden(kind string, forbidden, actual []string) []string {
	present := make(map[string]bool, len(actual))
	for _, value := range actual {
		present[value] = true
	}
	violations := make([]string, 0)
	for _, value := range forbidden {
		if present[value] {
			violations = append(violations, fmt.Sprintf("forbidden %s %q", kind, value))
		}
	}
	return violations
}

func safeID(value string) bool {
	return value != "" && value != "." && value != ".." && filepath.Base(value) == value && !strings.ContainsAny(value, `/\\`)
}

type directoryFS string

func osDirFS(dir string) fs.FS {
	return directoryFS(dir)
}

func (root directoryFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, fs.ErrInvalid
	}
	return openFile(filepath.Join(string(root), filepath.FromSlash(name)))
}
