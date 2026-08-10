// Package curator creates approval-gated learning candidates from finalized runs.
package curator

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/kyago/pylon/internal/corpus"
	"github.com/kyago/pylon/internal/fsutil"
	"github.com/kyago/pylon/internal/history"
	"github.com/kyago/pylon/internal/layout"
)

const SchemaVersion = 1

var (
	ErrNotFinalized      = errors.New("run is not finalized for curator input")
	ErrInvalidProposal   = errors.New("invalid curator proposal")
	ErrInvalidTransition = errors.New("invalid curator candidate transition")
)

var allowedTypes = map[string]bool{
	"memory": true, "pitfall": true, "skill": true, "agent_prompt": true,
	"pipeline_rule": true, "acceptance_corpus": true,
}

type Proposal struct {
	Type               string   `json:"type"`
	Title              string   `json:"title"`
	Summary            string   `json:"summary"`
	Rationale          string   `json:"rationale"`
	TargetFiles        []string `json:"target_files"`
	EvidenceRefs       []string `json:"evidence_refs,omitempty"`
	RegressionFixtures []string `json:"regression_fixtures,omitempty"`
}

type EvidenceSource struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
}

type Evidence struct {
	SchemaVersion    int              `json:"schema_version"`
	CheckpointRef    string           `json:"checkpoint_ref"`
	CheckpointDigest string           `json:"checkpoint_digest"`
	Sources          []EvidenceSource `json:"sources"`
}

type SourceRun struct {
	PipelineID string        `json:"pipeline_id"`
	Phase      history.Phase `json:"phase"`
	Ref        string        `json:"ref"`
	Digest     string        `json:"digest"`
	RecordedAt time.Time     `json:"recorded_at"`
}

type Decision struct {
	Decision string    `json:"decision"`
	Reason   string    `json:"reason"`
	At       time.Time `json:"at"`
}

type RegressionGate struct {
	ReportDigest     string    `json:"report_digest"`
	FixtureSetDigest string    `json:"fixture_set_digest"`
	CaseCount        int       `json:"case_count"`
	PassedAt         time.Time `json:"passed_at"`
}

type Status struct {
	SchemaVersion      int               `json:"schema_version"`
	CandidateID        string            `json:"candidate_id"`
	Type               string            `json:"type"`
	Status             string            `json:"status"`
	SourceCheckpoint   string            `json:"source_checkpoint"`
	SourceDigest       string            `json:"source_digest"`
	ProposalDigest     string            `json:"proposal_digest"`
	Artifacts          map[string]string `json:"artifacts"`
	CreatedAt          time.Time         `json:"created_at"`
	UpdatedAt          time.Time         `json:"updated_at"`
	Decisions          []Decision        `json:"decisions,omitempty"`
	RegressionFixtures []string          `json:"regression_fixtures,omitempty"`
	RegressionGate     *RegressionGate   `json:"regression_gate,omitempty"`
}

type Candidate struct {
	ID     string `json:"id"`
	Path   string `json:"path"`
	Status Status `json:"status"`
}

type CreateOptions struct {
	Root          string
	CheckpointRef string
	Proposal      Proposal
	Now           func() time.Time
}

func Create(options CreateOptions) (Candidate, error) {
	proposal := options.Proposal
	normalizeProposal(&proposal)
	if err := validateProposal(proposal); err != nil {
		return Candidate{}, err
	}
	fixtures, err := corpus.LoadEmbedded()
	if err != nil {
		return Candidate{}, err
	}
	if err := validateRegressionFixtures(proposal.RegressionFixtures, fixtures); err != nil {
		return Candidate{}, err
	}
	manager := history.NewManager(options.Root)
	manifest, err := manager.Validate(options.CheckpointRef)
	if err != nil {
		return Candidate{}, err
	}
	checkpointDir := filepath.Join(layout.HistoryDir(options.Root), "pipelines", manifest.PipelineID, string(manifest.Phase))
	if err := validateFinalizedCheckpoint(checkpointDir, *manifest); err != nil {
		return Candidate{}, err
	}
	proposalDigest, err := digestJSON(proposal)
	if err != nil {
		return Candidate{}, err
	}
	candidateID := candidateID(proposal, manifest.Digest, proposalDigest)
	candidatesDir := layout.LearningCandidatesDir(options.Root)
	if err := os.MkdirAll(candidatesDir, 0755); err != nil {
		return Candidate{}, err
	}
	unlock, err := fsutil.AcquireFileLock(filepath.Join(layout.LearningDir(options.Root), ".curator.lock"), 10*time.Second)
	if err != nil {
		return Candidate{}, err
	}
	defer unlock()
	target := filepath.Join(candidatesDir, candidateID)
	if existing, loadErr := loadStatus(filepath.Join(target, "status.json")); loadErr == nil {
		if existing.SourceDigest == manifest.Digest && existing.ProposalDigest == proposalDigest {
			return Candidate{ID: candidateID, Path: target, Status: existing}, nil
		}
		return Candidate{}, fmt.Errorf("candidate already exists with different content: %s", candidateID)
	} else if !os.IsNotExist(loadErr) {
		return Candidate{}, loadErr
	}

	now := resolveNow(options.Now)
	status := Status{
		SchemaVersion: SchemaVersion, CandidateID: candidateID, Type: proposal.Type,
		Status: "pending_review", SourceCheckpoint: options.CheckpointRef,
		SourceDigest: manifest.Digest, ProposalDigest: proposalDigest,
		RegressionFixtures: proposal.RegressionFixtures,
		CreatedAt:          now, UpdatedAt: now,
	}
	evidence, err := buildEvidence(checkpointDir, options.CheckpointRef, *manifest, proposal.EvidenceRefs)
	if err != nil {
		return Candidate{}, err
	}
	temp, err := os.MkdirTemp(candidatesDir, ".candidate-")
	if err != nil {
		return Candidate{}, err
	}
	defer os.RemoveAll(temp)
	if err := fsutil.WriteFileAtomic(filepath.Join(temp, "proposal.md"), []byte(renderProposal(proposal)), 0644); err != nil {
		return Candidate{}, err
	}
	if err := fsutil.WriteJSONAtomic(filepath.Join(temp, "evidence.json"), evidence); err != nil {
		return Candidate{}, err
	}
	if err := fsutil.WriteJSONAtomic(filepath.Join(temp, "source-runs.json"), []SourceRun{{
		PipelineID: manifest.PipelineID, Phase: manifest.Phase, Ref: options.CheckpointRef,
		Digest: manifest.Digest, RecordedAt: manifest.RecordedAt,
	}}); err != nil {
		return Candidate{}, err
	}
	if err := fsutil.WriteJSONAtomic(filepath.Join(temp, "target-files.json"), map[string]any{
		"files": proposal.TargetFiles, "regression_fixtures": proposal.RegressionFixtures,
	}); err != nil {
		return Candidate{}, err
	}
	status.Artifacts, err = candidateArtifacts(temp)
	if err != nil {
		return Candidate{}, err
	}
	if err := fsutil.WriteJSONAtomic(filepath.Join(temp, "status.json"), status); err != nil {
		return Candidate{}, err
	}
	if err := os.Rename(temp, target); err != nil {
		return Candidate{}, err
	}
	return Candidate{ID: candidateID, Path: target, Status: status}, nil
}

func Review(root, candidateIDValue, decision, reason string, now func() time.Time) (Status, error) {
	decision = strings.TrimSpace(decision)
	reason = strings.TrimSpace(reason)
	if decision != "approve" && decision != "reject" {
		return Status{}, fmt.Errorf("%w: decision must be approve or reject", ErrInvalidTransition)
	}
	if reason == "" {
		return Status{}, fmt.Errorf("%w: review reason is required", ErrInvalidTransition)
	}
	return updateCandidateStatus(root, candidateIDValue, func(status *Status) error {
		if status.Status != "pending_review" {
			return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, status.Status, decision)
		}
		status.Status = map[string]string{"approve": "approved", "reject": "rejected"}[decision]
		status.UpdatedAt = resolveNow(now)
		status.Decisions = append(status.Decisions, Decision{Decision: decision, Reason: reason, At: status.UpdatedAt})
		return nil
	})
}

func Gate(root, candidateIDValue, reportPath string, now func() time.Time) (Status, error) {
	data, err := os.ReadFile(reportPath)
	if err != nil {
		return Status{}, err
	}
	var report corpus.Report
	if err := json.Unmarshal(data, &report); err != nil {
		return Status{}, fmt.Errorf("corpus report parse failed: %w", err)
	}
	fixtures, err := corpus.LoadEmbedded()
	if err != nil {
		return Status{}, err
	}
	if err := corpus.ValidateReport(report, fixtures); err != nil {
		return Status{}, fmt.Errorf("%w: %v", ErrInvalidTransition, err)
	}
	caseIDs := make(map[string]bool, len(report.Cases))
	for _, result := range report.Cases {
		caseIDs[result.FixtureID] = true
	}
	reportDigest := digestBytes(data)
	return updateCandidateStatus(root, candidateIDValue, func(status *Status) error {
		if status.Status != "approved" {
			return fmt.Errorf("%w: %s -> regression_passed", ErrInvalidTransition, status.Status)
		}
		for _, fixtureID := range status.RegressionFixtures {
			if !caseIDs[fixtureID] {
				return fmt.Errorf("%w: required regression fixture %q was not run", ErrInvalidTransition, fixtureID)
			}
		}
		passedAt := resolveNow(now)
		status.Status = "regression_passed"
		status.UpdatedAt = passedAt
		status.RegressionGate = &RegressionGate{
			ReportDigest: reportDigest, FixtureSetDigest: report.FixtureSetDigest,
			CaseCount: len(report.Cases), PassedAt: passedAt,
		}
		return nil
	})
}

func validateRegressionFixtures(requested []string, fixtures []corpus.Fixture) error {
	known := make(map[string]bool, len(fixtures))
	for _, fixture := range fixtures {
		known[fixture.ID] = true
	}
	for _, fixtureID := range requested {
		if !known[fixtureID] {
			return fmt.Errorf("%w: unknown regression fixture %q", ErrInvalidProposal, fixtureID)
		}
	}
	return nil
}

func validateFinalizedCheckpoint(dir string, manifest history.Manifest) error {
	if manifest.Phase != history.PhaseCompleted && manifest.Phase != history.PhaseFailed {
		return fmt.Errorf("%w: checkpoint phase %s is not finalized", ErrNotFinalized, manifest.Phase)
	}
	required := []string{
		"criteria-summary.json", "verification-summary.json", "evaluator-summary.json",
		"task-reports-summary.json", "failure-records-summary.json",
	}
	for _, name := range required {
		if _, ok := manifest.Artifacts[name]; !ok {
			return fmt.Errorf("%w: missing %s", ErrNotFinalized, name)
		}
	}
	criteria, err := summaryResults(filepath.Join(dir, "criteria-summary.json"))
	if err != nil || len(criteria) == 0 {
		return fmt.Errorf("%w: criteria summary is incomplete", ErrNotFinalized)
	}
	for _, result := range criteria {
		if strings.TrimSpace(stringValue(result["digest"])) == "" {
			return fmt.Errorf("%w: criteria digest is missing", ErrNotFinalized)
		}
	}
	verification, err := summaryResults(filepath.Join(dir, "verification-summary.json"))
	if err != nil || len(verification) == 0 {
		return fmt.Errorf("%w: deterministic verification is missing", ErrNotFinalized)
	}
	evaluations, err := summaryResults(filepath.Join(dir, "evaluator-summary.json"))
	if err != nil || len(evaluations) == 0 {
		return fmt.Errorf("%w: evaluator verdict is missing", ErrNotFinalized)
	}
	for _, evaluation := range evaluations {
		status := stringValue(evaluation["status"])
		if status != "pass" && status != "fail" {
			return fmt.Errorf("%w: evaluator verdict is not final: %q", ErrNotFinalized, status)
		}
		if manifest.Phase == history.PhaseCompleted && status != "pass" {
			return fmt.Errorf("%w: completed run has non-pass evaluator verdict", ErrNotFinalized)
		}
	}
	for _, result := range verification {
		passed, ok := result["ok"].(bool)
		if !ok {
			return fmt.Errorf("%w: verification result is not final", ErrNotFinalized)
		}
		if manifest.Phase == history.PhaseCompleted && !passed {
			return fmt.Errorf("%w: completed run has failed verification", ErrNotFinalized)
		}
	}
	taskReports, err := summaryResults(filepath.Join(dir, "task-reports-summary.json"))
	if err != nil || len(taskReports) == 0 {
		return fmt.Errorf("%w: task reports are missing", ErrNotFinalized)
	}
	failures, err := summaryResults(filepath.Join(dir, "failure-records-summary.json"))
	if err != nil {
		return fmt.Errorf("%w: failure record collection is missing", ErrNotFinalized)
	}
	if manifest.Phase == history.PhaseFailed && len(failures) == 0 {
		return fmt.Errorf("%w: failed run has no failure record", ErrNotFinalized)
	}
	return nil
}

func buildEvidence(dir, ref string, manifest history.Manifest, requested []string) (Evidence, error) {
	paths := append([]string{
		"criteria-summary.json", "verification-summary.json", "evaluator-summary.json",
		"task-reports-summary.json", "failure-records-summary.json",
	}, requested...)
	paths = normalizeStrings(paths)
	sources := make([]EvidenceSource, 0, len(paths))
	for _, path := range paths {
		if err := validateRelativePath(path); err != nil {
			return Evidence{}, err
		}
		digest, ok := manifest.Artifacts[filepath.ToSlash(path)]
		if !ok {
			return Evidence{}, fmt.Errorf("%w: evidence is not in checkpoint: %s", ErrInvalidProposal, path)
		}
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(path))); err != nil {
			return Evidence{}, err
		}
		sources = append(sources, EvidenceSource{Path: filepath.ToSlash(path), Digest: digest})
	}
	return Evidence{SchemaVersion: SchemaVersion, CheckpointRef: ref, CheckpointDigest: manifest.Digest, Sources: sources}, nil
}

func updateCandidateStatus(root, candidateIDValue string, mutate func(*Status) error) (Status, error) {
	if !safeID(candidateIDValue) {
		return Status{}, fmt.Errorf("invalid candidate id: %q", candidateIDValue)
	}
	unlock, err := fsutil.AcquireFileLock(filepath.Join(layout.LearningDir(root), ".curator.lock"), 10*time.Second)
	if err != nil {
		return Status{}, err
	}
	defer unlock()
	path := filepath.Join(layout.LearningCandidatesDir(root), candidateIDValue, "status.json")
	status, err := loadStatus(path)
	if err != nil {
		return Status{}, err
	}
	if err := verifyCandidateArtifacts(filepath.Dir(path), status); err != nil {
		return Status{}, err
	}
	if err := mutate(&status); err != nil {
		return Status{}, err
	}
	if err := fsutil.WriteJSONAtomic(path, status); err != nil {
		return Status{}, err
	}
	return status, nil
}

func summaryResults(path string) ([]map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, err
	}
	if recordsValue, ok := value["records"]; ok {
		records, ok := recordsValue.([]any)
		if !ok {
			return nil, fmt.Errorf("invalid summary records: %s", path)
		}
		results := make([]map[string]any, 0, len(records))
		for _, item := range records {
			record, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid summary record: %s", path)
			}
			result, ok := record["result"].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid summary result: %s", path)
			}
			results = append(results, result)
		}
		return results, nil
	}
	return []map[string]any{value}, nil
}

func renderProposal(proposal Proposal) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "# %s\n\n", proposal.Title)
	fmt.Fprintf(&builder, "- Type: `%s`\n", proposal.Type)
	fmt.Fprintf(&builder, "- Status: candidate only; no active files modified\n\n")
	fmt.Fprintf(&builder, "## Summary\n\n%s\n\n", proposal.Summary)
	fmt.Fprintf(&builder, "## Rationale\n\n%s\n\n", proposal.Rationale)
	builder.WriteString("## Target Files\n\n")
	for _, path := range proposal.TargetFiles {
		fmt.Fprintf(&builder, "- `%s`\n", path)
	}
	if len(proposal.RegressionFixtures) > 0 {
		builder.WriteString("\n## Regression Fixtures\n\n")
		for _, fixture := range proposal.RegressionFixtures {
			fmt.Fprintf(&builder, "- `%s`\n", fixture)
		}
	}
	return builder.String()
}

func validateProposal(proposal Proposal) error {
	if !allowedTypes[proposal.Type] {
		return fmt.Errorf("%w: unsupported type %q", ErrInvalidProposal, proposal.Type)
	}
	if proposal.Title == "" || proposal.Summary == "" || proposal.Rationale == "" || len(proposal.TargetFiles) == 0 {
		return fmt.Errorf("%w: title, summary, rationale, and target files are required", ErrInvalidProposal)
	}
	for _, path := range append(append([]string(nil), proposal.TargetFiles...), proposal.EvidenceRefs...) {
		if err := validateRelativePath(path); err != nil {
			return err
		}
	}
	return nil
}

func normalizeProposal(proposal *Proposal) {
	proposal.Type = strings.TrimSpace(proposal.Type)
	proposal.Title = strings.TrimSpace(proposal.Title)
	proposal.Summary = strings.TrimSpace(proposal.Summary)
	proposal.Rationale = strings.TrimSpace(proposal.Rationale)
	proposal.TargetFiles = normalizeStrings(proposal.TargetFiles)
	proposal.EvidenceRefs = normalizeStrings(proposal.EvidenceRefs)
	proposal.RegressionFixtures = normalizeStrings(proposal.RegressionFixtures)
}

func normalizeStrings(values []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func candidateID(proposal Proposal, sourceDigest, proposalDigest string) string {
	base := slug(proposal.Title)
	if base == "" {
		base = strings.ReplaceAll(proposal.Type, "_", "-")
	}
	sum := sha256.Sum256([]byte(sourceDigest + "\x00" + proposalDigest))
	return base + "-" + hex.EncodeToString(sum[:4])
}

func slug(value string) string {
	var builder strings.Builder
	lastDash := false
	for _, character := range strings.ToLower(value) {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			builder.WriteRune(character)
			lastDash = false
		} else if !lastDash && builder.Len() > 0 {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(builder.String(), "-")
}

func validateRelativePath(path string) error {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "" || clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%w: unsafe path %q", ErrInvalidProposal, path)
	}
	return nil
}

func safeID(value string) bool {
	return value != "" && value != "." && value != ".." && filepath.Base(value) == value && !strings.ContainsAny(value, `/\\`)
}

func digestJSON(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return digestBytes(data), nil
}

func digestBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func loadStatus(path string) (Status, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Status{}, err
	}
	var status Status
	if err := json.Unmarshal(data, &status); err != nil {
		return Status{}, err
	}
	if status.SchemaVersion != SchemaVersion || !safeID(status.CandidateID) || len(status.Artifacts) == 0 {
		return Status{}, fmt.Errorf("invalid candidate status: %s", path)
	}
	return status, nil
}

func candidateArtifacts(dir string) (map[string]string, error) {
	artifacts := make(map[string]string)
	for _, name := range []string{"proposal.md", "evidence.json", "source-runs.json", "target-files.json"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		artifacts[name] = digestBytes(data)
	}
	return artifacts, nil
}

func verifyCandidateArtifacts(dir string, status Status) error {
	actual, err := candidateArtifacts(dir)
	if err != nil {
		return err
	}
	if len(actual) != len(status.Artifacts) {
		return fmt.Errorf("candidate artifact count mismatch")
	}
	for name, expected := range status.Artifacts {
		if actual[name] != expected {
			return fmt.Errorf("candidate artifact integrity failure: %s", name)
		}
	}
	return nil
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func resolveNow(now func() time.Time) time.Time {
	if now == nil {
		return time.Now().UTC()
	}
	return now().UTC()
}
