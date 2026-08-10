// Package trajectory records provider-neutral task reports and terminal failures.
package trajectory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/kyago/pylon/internal/fsutil"
)

const SchemaVersion = 1

var (
	ErrAlreadyExists    = errors.New("trajectory artifact already exists")
	ErrIntegrityFailure = errors.New("trajectory artifact integrity failure")
	ErrInvalidArtifact  = errors.New("invalid trajectory artifact")
)

type RejectedHypothesis struct {
	Hypothesis string `json:"hypothesis"`
	Probe      string `json:"probe"`
	Result     string `json:"result"`
}

type ProviderSummary struct {
	Name         string   `json:"name"`
	ExternalID   string   `json:"external_id,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
}

type VerificationSummary struct {
	CriteriaDigest      string   `json:"criteria_digest,omitempty"`
	DeterministicPassed bool     `json:"deterministic_passed"`
	EvaluatorStatus     string   `json:"evaluator_status,omitempty"`
	EvidenceRefs        []string `json:"evidence_refs,omitempty"`
}

type TaskReport struct {
	SchemaVersion        int                  `json:"schema_version"`
	RunID                string               `json:"run_id"`
	TaskID               string               `json:"task_id"`
	RepoID               string               `json:"repo_id,omitempty"`
	Attempt              int                  `json:"attempt"`
	Status               string               `json:"status"`
	Summary              string               `json:"summary"`
	Provider             *ProviderSummary     `json:"provider,omitempty"`
	RequiredCapabilities []string             `json:"required_capabilities,omitempty"`
	ChangedFiles         []string             `json:"changed_files,omitempty"`
	EvidenceRefs         []string             `json:"evidence_refs,omitempty"`
	Verification         *VerificationSummary `json:"verification,omitempty"`
	HypothesesRejected   []RejectedHypothesis `json:"hypotheses_rejected,omitempty"`
	RemainingUnknowns    []string             `json:"remaining_unknowns,omitempty"`
	StartedAt            time.Time            `json:"started_at,omitempty"`
	CompletedAt          time.Time            `json:"completed_at"`
	Digest               string               `json:"digest"`
}

type FailureRecord struct {
	SchemaVersion      int                  `json:"schema_version"`
	RunID              string               `json:"run_id"`
	TaskID             string               `json:"task_id"`
	RepoID             string               `json:"repo_id,omitempty"`
	Attempt            int                  `json:"attempt"`
	Phase              string               `json:"phase"`
	TerminalCause      string               `json:"terminal_cause"`
	EvidenceRefs       []string             `json:"evidence_refs,omitempty"`
	HypothesesRejected []RejectedHypothesis `json:"hypotheses_rejected,omitempty"`
	RemainingUnknowns  []string             `json:"remaining_unknowns,omitempty"`
	AbandonedReason    string               `json:"abandoned_reason"`
	RecordedAt         time.Time            `json:"recorded_at"`
	Digest             string               `json:"digest"`
}

type RecordOptions struct {
	OutputPath string
	Now        func() time.Time
}

func RecordTaskReport(report TaskReport, options RecordOptions) (TaskReport, error) {
	now := resolveNow(options.Now)
	report.SchemaVersion = SchemaVersion
	if report.CompletedAt.IsZero() {
		report.CompletedAt = now
	}
	report.CompletedAt = report.CompletedAt.UTC()
	if !report.StartedAt.IsZero() {
		report.StartedAt = report.StartedAt.UTC()
	}
	normalizeTaskReport(&report)
	if err := validateTaskReport(report); err != nil {
		return TaskReport{}, err
	}
	digest, err := digestValue(report)
	if err != nil {
		return TaskReport{}, err
	}
	report.Digest = digest
	if err := writeCreateOnce(options.OutputPath, report, func(path string) (any, error) {
		return LoadTaskReport(path)
	}); err != nil {
		return TaskReport{}, err
	}
	return report, nil
}

func RecordFailure(record FailureRecord, options RecordOptions) (FailureRecord, error) {
	record.SchemaVersion = SchemaVersion
	if record.RecordedAt.IsZero() {
		record.RecordedAt = resolveNow(options.Now)
	}
	record.RecordedAt = record.RecordedAt.UTC()
	normalizeFailure(&record)
	if err := validateFailure(record); err != nil {
		return FailureRecord{}, err
	}
	digest, err := digestValue(record)
	if err != nil {
		return FailureRecord{}, err
	}
	record.Digest = digest
	if err := writeCreateOnce(options.OutputPath, record, func(path string) (any, error) {
		return LoadFailure(path)
	}); err != nil {
		return FailureRecord{}, err
	}
	return record, nil
}

func LoadTaskReport(path string) (TaskReport, error) {
	report, err := loadJSON[TaskReport](path)
	if err != nil {
		return TaskReport{}, err
	}
	if err := validateTaskReport(report); err != nil {
		return TaskReport{}, err
	}
	expected, err := digestValue(report)
	if err != nil {
		return TaskReport{}, err
	}
	if report.Digest == "" || report.Digest != expected {
		return TaskReport{}, fmt.Errorf("%w: %s", ErrIntegrityFailure, path)
	}
	return report, nil
}

func LoadFailure(path string) (FailureRecord, error) {
	record, err := loadJSON[FailureRecord](path)
	if err != nil {
		return FailureRecord{}, err
	}
	if err := validateFailure(record); err != nil {
		return FailureRecord{}, err
	}
	expected, err := digestValue(record)
	if err != nil {
		return FailureRecord{}, err
	}
	if record.Digest == "" || record.Digest != expected {
		return FailureRecord{}, fmt.Errorf("%w: %s", ErrIntegrityFailure, path)
	}
	return record, nil
}

func validateTaskReport(report TaskReport) error {
	if report.SchemaVersion != SchemaVersion {
		return invalid("unsupported task report schema version: %d", report.SchemaVersion)
	}
	if err := validateID("run", report.RunID); err != nil {
		return err
	}
	if err := validateID("task", report.TaskID); err != nil {
		return err
	}
	if report.Attempt < 1 {
		return invalid("attempt must be positive")
	}
	switch report.Status {
	case "succeeded", "failed", "cancelled", "interrupted":
	default:
		return invalid("task report status must be terminal: %q", report.Status)
	}
	if strings.TrimSpace(report.Summary) == "" {
		return invalid("task report summary is required")
	}
	if report.CompletedAt.IsZero() {
		return invalid("task report completed_at is required")
	}
	if !report.StartedAt.IsZero() && report.StartedAt.After(report.CompletedAt) {
		return invalid("task report started_at is after completed_at")
	}
	if err := validateRefs(report.ChangedFiles); err != nil {
		return err
	}
	if err := validateRefs(report.EvidenceRefs); err != nil {
		return err
	}
	if report.Verification != nil {
		if err := validateRefs(report.Verification.EvidenceRefs); err != nil {
			return err
		}
	}
	return validateHypotheses(report.HypothesesRejected)
}

func validateFailure(record FailureRecord) error {
	if record.SchemaVersion != SchemaVersion {
		return invalid("unsupported failure record schema version: %d", record.SchemaVersion)
	}
	if err := validateID("run", record.RunID); err != nil {
		return err
	}
	if err := validateID("task", record.TaskID); err != nil {
		return err
	}
	if record.Attempt < 1 {
		return invalid("attempt must be positive")
	}
	if strings.TrimSpace(record.Phase) == "" || strings.TrimSpace(record.TerminalCause) == "" {
		return invalid("failure phase and terminal cause are required")
	}
	if strings.TrimSpace(record.AbandonedReason) == "" {
		return invalid("abandoned reason is required")
	}
	if record.RecordedAt.IsZero() {
		return invalid("failure recorded_at is required")
	}
	if err := validateRefs(record.EvidenceRefs); err != nil {
		return err
	}
	return validateHypotheses(record.HypothesesRejected)
}

func validateHypotheses(hypotheses []RejectedHypothesis) error {
	for index, hypothesis := range hypotheses {
		if strings.TrimSpace(hypothesis.Hypothesis) == "" || strings.TrimSpace(hypothesis.Probe) == "" || strings.TrimSpace(hypothesis.Result) == "" {
			return invalid("rejected hypothesis %d is incomplete", index+1)
		}
	}
	return nil
}

func validateRefs(refs []string) error {
	for _, reference := range refs {
		clean := filepath.Clean(strings.TrimSpace(reference))
		if clean == "." || clean == "" || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return invalid("unsafe artifact reference: %q", reference)
		}
	}
	return nil
}

func validateID(kind, value string) error {
	value = strings.TrimSpace(value)
	if value == "" || value == "." || value == ".." || filepath.Base(value) != value || strings.ContainsAny(value, `/\\`) {
		return invalid("invalid %s id: %q", kind, value)
	}
	return nil
}

func normalizeTaskReport(report *TaskReport) {
	report.RunID = strings.TrimSpace(report.RunID)
	report.TaskID = strings.TrimSpace(report.TaskID)
	report.RepoID = strings.TrimSpace(report.RepoID)
	report.Status = strings.TrimSpace(report.Status)
	report.Summary = strings.TrimSpace(report.Summary)
	report.RequiredCapabilities = normalizeStrings(report.RequiredCapabilities)
	report.ChangedFiles = normalizeStrings(report.ChangedFiles)
	report.EvidenceRefs = normalizeStrings(report.EvidenceRefs)
	report.RemainingUnknowns = normalizeStrings(report.RemainingUnknowns)
	if report.Provider != nil {
		report.Provider.Name = strings.TrimSpace(report.Provider.Name)
		report.Provider.ExternalID = strings.TrimSpace(report.Provider.ExternalID)
		report.Provider.Capabilities = normalizeStrings(report.Provider.Capabilities)
	}
	if report.Verification != nil {
		report.Verification.CriteriaDigest = strings.TrimSpace(report.Verification.CriteriaDigest)
		report.Verification.EvaluatorStatus = strings.TrimSpace(report.Verification.EvaluatorStatus)
		report.Verification.EvidenceRefs = normalizeStrings(report.Verification.EvidenceRefs)
	}
	normalizeHypotheses(report.HypothesesRejected)
}

func normalizeFailure(record *FailureRecord) {
	record.RunID = strings.TrimSpace(record.RunID)
	record.TaskID = strings.TrimSpace(record.TaskID)
	record.RepoID = strings.TrimSpace(record.RepoID)
	record.Phase = strings.TrimSpace(record.Phase)
	record.TerminalCause = strings.TrimSpace(record.TerminalCause)
	record.AbandonedReason = strings.TrimSpace(record.AbandonedReason)
	record.EvidenceRefs = normalizeStrings(record.EvidenceRefs)
	record.RemainingUnknowns = normalizeStrings(record.RemainingUnknowns)
	normalizeHypotheses(record.HypothesesRejected)
}

func normalizeHypotheses(hypotheses []RejectedHypothesis) {
	for index := range hypotheses {
		hypotheses[index].Hypothesis = strings.TrimSpace(hypotheses[index].Hypothesis)
		hypotheses[index].Probe = strings.TrimSpace(hypotheses[index].Probe)
		hypotheses[index].Result = strings.TrimSpace(hypotheses[index].Result)
	}
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
	if len(result) == 0 {
		return nil
	}
	sort.Strings(result)
	return result
}

func writeCreateOnce(path string, value any, load func(string) (any, error)) error {
	if strings.TrimSpace(path) == "" {
		return invalid("trajectory output path is required")
	}
	unlock, err := fsutil.AcquireFileLock(filepath.Join(filepath.Dir(path), ".trajectory.lock"), 10*time.Second)
	if err != nil {
		return err
	}
	defer unlock()
	if existing, loadErr := load(path); loadErr == nil {
		if reflect.DeepEqual(existing, value) {
			return nil
		}
		return fmt.Errorf("%w: %s", ErrAlreadyExists, path)
	} else if !os.IsNotExist(loadErr) {
		return loadErr
	}
	return fsutil.WriteJSONAtomic(path, value)
}

func digestValue(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	var generic map[string]any
	if err := json.Unmarshal(data, &generic); err != nil {
		return "", err
	}
	generic["digest"] = ""
	canonical, err := json.Marshal(generic)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func loadJSON[T any](path string) (T, error) {
	var value T
	data, err := os.ReadFile(path)
	if err != nil {
		return value, err
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return value, fmt.Errorf("%s JSON parse failed: %w", path, err)
	}
	return value, nil
}

func resolveNow(now func() time.Time) time.Time {
	if now == nil {
		return time.Now().UTC()
	}
	return now().UTC()
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidArtifact, fmt.Sprintf(format, args...))
}
