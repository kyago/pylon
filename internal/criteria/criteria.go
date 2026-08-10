// Package criteria creates and validates immutable verification criteria snapshots.
package criteria

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/fsutil"
	"github.com/kyago/pylon/internal/provider"
)

const SchemaVersion = 1

var (
	ErrNoVerification   = errors.New("no verification commands configured")
	ErrIntegrityFailure = errors.New("criteria snapshot integrity failure")
	ErrAlreadyExists    = errors.New("criteria snapshot already exists")
)

type Source struct {
	Path   string `json:"path"`
	Mode   string `json:"mode"`
	Digest string `json:"digest,omitempty"`
}

type Snapshot struct {
	SchemaVersion      int                      `json:"schema_version"`
	RunID              string                   `json:"run_id"`
	RepoID             string                   `json:"repo_id"`
	BaseRevision       string                   `json:"base_revision"`
	AcceptanceCriteria []provider.Criterion     `json:"acceptance_criteria,omitempty"`
	Verification       []config.NamedVerifyStep `json:"verification"`
	Source             Source                   `json:"source"`
	CreatedAt          time.Time                `json:"created_at"`
	Digest             string                   `json:"digest"`
}

type HeldOutSnapshot struct {
	SchemaVersion  int                      `json:"schema_version"`
	RunID          string                   `json:"run_id"`
	RepoID         string                   `json:"repo_id"`
	BaseRevision   string                   `json:"base_revision"`
	CriteriaDigest string                   `json:"criteria_digest"`
	Verification   []config.NamedVerifyStep `json:"verification"`
	Source         Source                   `json:"source"`
	CreatedAt      time.Time                `json:"created_at"`
	Digest         string                   `json:"digest"`
}

type ManifestReference struct {
	Path         string    `json:"path"`
	Digest       string    `json:"digest"`
	SourceDigest string    `json:"source_digest,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type Event struct {
	SchemaVersion int       `json:"schema_version"`
	Type          string    `json:"type"`
	Expected      string    `json:"expected,omitempty"`
	Actual        string    `json:"actual,omitempty"`
	Message       string    `json:"message"`
	RecordedAt    time.Time `json:"recorded_at"`
}

type CreateOptions struct {
	RunID              string
	RepoID             string
	BaseRevision       string
	WorkDir            string
	ConfigPath         string
	AcceptancePath     string
	OutputPath         string
	HeldOutOutputPath  string
	ManifestPath       string
	AcceptanceCriteria []provider.Criterion
	Now                func() time.Time
}

func Create(options CreateOptions) (Snapshot, error) {
	if strings.TrimSpace(options.RunID) == "" {
		return Snapshot{}, errors.New("run id is required")
	}
	if strings.TrimSpace(options.RepoID) == "" {
		return Snapshot{}, errors.New("repo id is required")
	}
	if strings.TrimSpace(options.BaseRevision) == "" {
		return Snapshot{}, errors.New("base revision is required")
	}
	if options.OutputPath == "" {
		return Snapshot{}, errors.New("criteria output path is required")
	}
	lockRoot := filepath.Dir(options.OutputPath)
	if options.ManifestPath != "" {
		lockRoot = filepath.Dir(options.ManifestPath)
	}
	unlock, err := fsutil.AcquireFileLock(filepath.Join(lockRoot, ".criteria.lock"), 10*time.Second)
	if err != nil {
		return Snapshot{}, err
	}
	defer unlock()
	steps, skipped, source, err := ResolveVerification(options.WorkDir, options.ConfigPath)
	if err != nil {
		return Snapshot{}, err
	}
	if skipped || len(steps) == 0 {
		return Snapshot{}, fmt.Errorf("%w: %s", ErrNoVerification, source.Path)
	}
	acceptance := normalizeCriteria(options.AcceptanceCriteria)
	if options.AcceptancePath != "" {
		acceptance, err = LoadAcceptanceCriteria(options.AcceptancePath)
		if err != nil {
			return Snapshot{}, err
		}
	}
	if err := validateAcceptanceCriteria(acceptance); err != nil {
		return Snapshot{}, err
	}
	if err := validateSteps(steps); err != nil {
		return Snapshot{}, err
	}
	deterministicSteps, heldOutSteps := splitVerificationSteps(steps)
	if len(heldOutSteps) > 0 && options.HeldOutOutputPath == "" {
		return Snapshot{}, errors.New("held-out output path is required when held-out verification is configured")
	}
	if existing, loadErr := Load(options.OutputPath); loadErr == nil {
		if existing.RunID == options.RunID &&
			existing.RepoID == options.RepoID &&
			existing.BaseRevision == options.BaseRevision &&
			reflect.DeepEqual(existing.AcceptanceCriteria, acceptance) &&
			reflect.DeepEqual(existing.Verification, deterministicSteps) &&
			existing.Source == source {
			if err := ensureHeldOutSnapshot(options, existing, heldOutSteps); err != nil {
				return Snapshot{}, err
			}
			if options.ManifestPath != "" {
				if err := BindManifest(options.ManifestPath, options.OutputPath, existing); err != nil {
					return Snapshot{}, err
				}
				if err := ValidateManifest(options.ManifestPath, options.OutputPath, existing); err != nil {
					return Snapshot{}, err
				}
			}
			return existing, nil
		}
		return Snapshot{}, fmt.Errorf("%w: %s", ErrAlreadyExists, options.OutputPath)
	} else if !os.IsNotExist(loadErr) {
		return Snapshot{}, loadErr
	}
	now := time.Now
	if options.Now != nil {
		now = options.Now
	}
	snapshot := Snapshot{
		SchemaVersion:      SchemaVersion,
		RunID:              options.RunID,
		RepoID:             options.RepoID,
		BaseRevision:       options.BaseRevision,
		AcceptanceCriteria: acceptance,
		Verification:       cloneSteps(deterministicSteps),
		Source:             source,
		CreatedAt:          now().UTC(),
	}
	digest, err := SnapshotDigest(snapshot)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot.Digest = digest
	if err := ensureHeldOutSnapshot(options, snapshot, heldOutSteps); err != nil {
		return Snapshot{}, err
	}
	if err := fsutil.WriteJSONAtomic(options.OutputPath, snapshot); err != nil {
		return Snapshot{}, err
	}
	if options.ManifestPath != "" {
		if err := BindManifest(options.ManifestPath, options.OutputPath, snapshot); err != nil {
			return Snapshot{}, err
		}
	}
	return snapshot, nil
}

func ensureHeldOutSnapshot(options CreateOptions, snapshot Snapshot, steps []config.NamedVerifyStep) error {
	if len(steps) == 0 {
		return nil
	}
	now := snapshot.CreatedAt
	heldOut := HeldOutSnapshot{
		SchemaVersion: SchemaVersion, RunID: snapshot.RunID, RepoID: snapshot.RepoID,
		BaseRevision: snapshot.BaseRevision, CriteriaDigest: snapshot.Digest,
		Verification: cloneSteps(steps), Source: snapshot.Source, CreatedAt: now,
	}
	digest, err := HeldOutDigest(heldOut)
	if err != nil {
		return err
	}
	heldOut.Digest = digest
	if existing, loadErr := LoadHeldOut(options.HeldOutOutputPath); loadErr == nil {
		if !reflect.DeepEqual(existing, heldOut) {
			return fmt.Errorf("%w: %s", ErrAlreadyExists, options.HeldOutOutputPath)
		}
	} else if os.IsNotExist(loadErr) {
		if err := fsutil.WriteJSONAtomic(options.HeldOutOutputPath, heldOut); err != nil {
			return err
		}
	} else {
		return loadErr
	}
	if options.ManifestPath != "" {
		if err := BindHeldOutManifest(options.ManifestPath, options.HeldOutOutputPath, heldOut); err != nil {
			return err
		}
		if err := ValidateHeldOutManifest(options.ManifestPath, options.HeldOutOutputPath, heldOut); err != nil {
			return err
		}
	}
	return nil
}

func ResolveVerification(workDir, configPath string) ([]config.NamedVerifyStep, bool, Source, error) {
	if configPath == "" {
		configPath = filepath.Join(workDir, ".pylon", "verify.yml")
	}
	if data, err := os.ReadFile(configPath); err == nil {
		verifyConfig, loadErr := config.LoadVerifyConfig(configPath)
		if loadErr != nil {
			return nil, false, Source{}, loadErr
		}
		steps := append(verifyConfig.OrderedSteps(), verifyConfig.OrderedHeldOutSteps()...)
		return steps, false, Source{Path: configPath, Mode: "verify_config", Digest: digestBytes(data)}, nil
	} else if !os.IsNotExist(err) {
		return nil, false, Source{}, fmt.Errorf("failed to inspect verify config: %w", err)
	}

	if _, err := os.Stat(filepath.Join(workDir, "go.mod")); err == nil {
		return []config.NamedVerifyStep{
			{Name: "build", Kind: config.VerifyKindDeterministic, Command: "go build ./...", Timeout: "5m"},
			{Name: "vet", Kind: config.VerifyKindDeterministic, Command: "go vet ./...", Timeout: "5m"},
			{Name: "test", Kind: config.VerifyKindDeterministic, Command: "go test ./...", Timeout: "10m"},
		}, false, Source{Path: configPath, Mode: "go_defaults"}, nil
	} else if !os.IsNotExist(err) {
		return nil, false, Source{}, fmt.Errorf("failed to inspect Go project: %w", err)
	}
	return nil, true, Source{Path: configPath, Mode: "missing"}, nil
}

func splitVerificationSteps(steps []config.NamedVerifyStep) ([]config.NamedVerifyStep, []config.NamedVerifyStep) {
	deterministic := make([]config.NamedVerifyStep, 0, len(steps))
	heldOut := make([]config.NamedVerifyStep, 0)
	for _, step := range steps {
		if step.Kind == config.VerifyKindHeldOut {
			heldOut = append(heldOut, step)
		} else {
			deterministic = append(deterministic, step)
		}
	}
	return deterministic, heldOut
}

func Load(path string) (Snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Snapshot{}, err
	}
	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return Snapshot{}, fmt.Errorf("failed to parse criteria snapshot: %w", err)
	}
	if snapshot.SchemaVersion != SchemaVersion {
		return Snapshot{}, fmt.Errorf("%w: unsupported schema version %d", ErrIntegrityFailure, snapshot.SchemaVersion)
	}
	actual, err := SnapshotDigest(snapshot)
	if err != nil {
		return Snapshot{}, err
	}
	if snapshot.Digest == "" || snapshot.Digest != actual {
		return Snapshot{}, fmt.Errorf("%w: snapshot digest mismatch (expected %s, actual %s)", ErrIntegrityFailure, snapshot.Digest, actual)
	}
	if err := validateSteps(snapshot.Verification); err != nil {
		return Snapshot{}, fmt.Errorf("%w: %v", ErrIntegrityFailure, err)
	}
	for _, step := range snapshot.Verification {
		if step.Kind == config.VerifyKindHeldOut {
			return Snapshot{}, fmt.Errorf("%w: held-out command is exposed in public criteria", ErrIntegrityFailure)
		}
	}
	if err := validateAcceptanceCriteria(snapshot.AcceptanceCriteria); err != nil {
		return Snapshot{}, fmt.Errorf("%w: %v", ErrIntegrityFailure, err)
	}
	if strings.TrimSpace(snapshot.RunID) == "" || strings.TrimSpace(snapshot.RepoID) == "" || strings.TrimSpace(snapshot.BaseRevision) == "" {
		return Snapshot{}, fmt.Errorf("%w: run_id, repo_id, and base_revision are required", ErrIntegrityFailure)
	}
	if snapshot.Source.Path == "" {
		return Snapshot{}, fmt.Errorf("%w: source path is required", ErrIntegrityFailure)
	}
	switch snapshot.Source.Mode {
	case "verify_config":
		if snapshot.Source.Digest == "" {
			return Snapshot{}, fmt.Errorf("%w: verify config source digest is required", ErrIntegrityFailure)
		}
	case "go_defaults", "missing":
		if snapshot.Source.Digest != "" {
			return Snapshot{}, fmt.Errorf("%w: fallback source must not have a digest", ErrIntegrityFailure)
		}
	default:
		return Snapshot{}, fmt.Errorf("%w: invalid source mode %q", ErrIntegrityFailure, snapshot.Source.Mode)
	}
	return snapshot, nil
}

func LoadHeldOut(path string) (HeldOutSnapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return HeldOutSnapshot{}, err
	}
	var snapshot HeldOutSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return HeldOutSnapshot{}, fmt.Errorf("failed to parse held-out snapshot: %w", err)
	}
	actual, err := HeldOutDigest(snapshot)
	if err != nil {
		return HeldOutSnapshot{}, err
	}
	if snapshot.SchemaVersion != SchemaVersion || snapshot.Digest == "" || snapshot.Digest != actual {
		return HeldOutSnapshot{}, fmt.Errorf("%w: held-out snapshot digest mismatch", ErrIntegrityFailure)
	}
	if snapshot.RunID == "" || snapshot.RepoID == "" || snapshot.BaseRevision == "" || snapshot.CriteriaDigest == "" {
		return HeldOutSnapshot{}, fmt.Errorf("%w: held-out identity is incomplete", ErrIntegrityFailure)
	}
	if len(snapshot.Verification) == 0 {
		return HeldOutSnapshot{}, fmt.Errorf("%w: held-out snapshot has no commands", ErrIntegrityFailure)
	}
	if err := validateSteps(snapshot.Verification); err != nil {
		return HeldOutSnapshot{}, fmt.Errorf("%w: %v", ErrIntegrityFailure, err)
	}
	for _, step := range snapshot.Verification {
		if step.Kind != config.VerifyKindHeldOut {
			return HeldOutSnapshot{}, fmt.Errorf("%w: non-held-out command in evaluator snapshot", ErrIntegrityFailure)
		}
	}
	return snapshot, nil
}

func SnapshotDigest(snapshot Snapshot) (string, error) {
	snapshot.Digest = ""
	data, err := json.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	return digestBytes(data), nil
}

func HeldOutDigest(snapshot HeldOutSnapshot) (string, error) {
	snapshot.Digest = ""
	data, err := json.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	return digestBytes(data), nil
}

func BindManifest(manifestPath, snapshotPath string, snapshot Snapshot) error {
	return bindManifestReference(manifestPath, snapshotPath, "criteria", ManifestReference{
		Digest: snapshot.Digest, SourceDigest: snapshot.Source.Digest, CreatedAt: snapshot.CreatedAt,
	})
}

func BindHeldOutManifest(manifestPath, snapshotPath string, snapshot HeldOutSnapshot) error {
	return bindManifestReference(manifestPath, snapshotPath, "held_out", ManifestReference{
		Digest: snapshot.Digest, SourceDigest: snapshot.Source.Digest, CreatedAt: snapshot.CreatedAt,
	})
}

func bindManifestReference(manifestPath, snapshotPath, field string, reference ManifestReference) error {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to read run manifest: %w", err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("failed to parse run manifest: %w", err)
	}
	relative, err := filepath.Rel(filepath.Dir(manifestPath), snapshotPath)
	if err != nil {
		return err
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("criteria snapshot must be inside the run manifest directory")
	}
	reference.Path = filepath.ToSlash(relative)
	if raw, exists := manifest[field]; exists && raw != nil {
		existingData, marshalErr := json.Marshal(raw)
		if marshalErr != nil {
			return marshalErr
		}
		var existing ManifestReference
		if unmarshalErr := json.Unmarshal(existingData, &existing); unmarshalErr != nil {
			return unmarshalErr
		}
		if existing != reference {
			return fmt.Errorf("%w: manifest %s reference", ErrAlreadyExists, field)
		}
		return nil
	}
	manifest[field] = reference
	return fsutil.WriteJSONAtomic(manifestPath, manifest)
}

func ValidateManifest(manifestPath, snapshotPath string, snapshot Snapshot) error {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("%w: failed to read run manifest: %v", ErrIntegrityFailure, err)
	}
	var manifest struct {
		PipelineID   string             `json:"pipeline_id"`
		RootRunID    string             `json:"root_pipeline_id"`
		RepoID       string             `json:"repo_id"`
		BaseRevision string             `json:"base_revision"`
		Criteria     *ManifestReference `json:"criteria"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("%w: failed to parse run manifest: %v", ErrIntegrityFailure, err)
	}
	if manifest.Criteria == nil {
		return fmt.Errorf("%w: manifest has no criteria reference", ErrIntegrityFailure)
	}
	expectedPath := filepath.Clean(filepath.Join(filepath.Dir(manifestPath), filepath.FromSlash(manifest.Criteria.Path)))
	actualPath, err := filepath.Abs(snapshotPath)
	if err != nil {
		return err
	}
	expectedPath, err = filepath.Abs(expectedPath)
	if err != nil {
		return err
	}
	if actualPath != expectedPath {
		return fmt.Errorf("%w: manifest criteria path mismatch", ErrIntegrityFailure)
	}
	if manifest.Criteria.Digest != snapshot.Digest {
		return fmt.Errorf("%w: manifest digest mismatch (expected %s, actual %s)", ErrIntegrityFailure, manifest.Criteria.Digest, snapshot.Digest)
	}
	if manifest.Criteria.SourceDigest != snapshot.Source.Digest {
		return fmt.Errorf("%w: manifest source digest mismatch", ErrIntegrityFailure)
	}
	if !manifest.Criteria.CreatedAt.Equal(snapshot.CreatedAt) {
		return fmt.Errorf("%w: manifest criteria timestamp mismatch", ErrIntegrityFailure)
	}
	manifestRunID := manifest.RootRunID
	if manifestRunID == "" {
		manifestRunID = manifest.PipelineID
	}
	if manifestRunID != "" && manifestRunID != snapshot.RunID {
		return fmt.Errorf("%w: run id mismatch", ErrIntegrityFailure)
	}
	if manifest.RepoID != "" && manifest.RepoID != snapshot.RepoID {
		return fmt.Errorf("%w: repo id mismatch", ErrIntegrityFailure)
	}
	if manifest.BaseRevision != "" && manifest.BaseRevision != snapshot.BaseRevision {
		return fmt.Errorf("%w: base revision mismatch", ErrIntegrityFailure)
	}
	return nil
}

func ValidateHeldOutManifest(manifestPath, snapshotPath string, snapshot HeldOutSnapshot) error {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("%w: failed to read run manifest: %v", ErrIntegrityFailure, err)
	}
	var manifest struct {
		RootRunID    string             `json:"root_pipeline_id"`
		PipelineID   string             `json:"pipeline_id"`
		RepoID       string             `json:"repo_id"`
		BaseRevision string             `json:"base_revision"`
		HeldOut      *ManifestReference `json:"held_out"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("%w: failed to parse run manifest: %v", ErrIntegrityFailure, err)
	}
	if manifest.HeldOut == nil {
		return fmt.Errorf("%w: manifest has no held-out reference", ErrIntegrityFailure)
	}
	if err := validateReferencePath(manifestPath, snapshotPath, *manifest.HeldOut, snapshot.Digest, snapshot.Source.Digest); err != nil {
		return err
	}
	runID := manifest.RootRunID
	if runID == "" {
		runID = manifest.PipelineID
	}
	if runID != "" && runID != snapshot.RunID {
		return fmt.Errorf("%w: manifest run id mismatch", ErrIntegrityFailure)
	}
	if manifest.RepoID != "" && manifest.RepoID != snapshot.RepoID {
		return fmt.Errorf("%w: manifest repo id mismatch", ErrIntegrityFailure)
	}
	if manifest.BaseRevision != "" && manifest.BaseRevision != snapshot.BaseRevision {
		return fmt.Errorf("%w: manifest base revision mismatch", ErrIntegrityFailure)
	}
	return nil
}

func ManifestHeldOutPath(manifestPath string) (string, bool, error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return "", false, err
	}
	var manifest struct {
		HeldOut *ManifestReference `json:"held_out"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "", false, err
	}
	if manifest.HeldOut == nil {
		return "", false, nil
	}
	clean := filepath.Clean(filepath.FromSlash(manifest.HeldOut.Path))
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.IsAbs(clean) {
		return "", false, fmt.Errorf("%w: held-out path escapes manifest directory", ErrIntegrityFailure)
	}
	return filepath.Join(filepath.Dir(manifestPath), clean), true, nil
}

func validateReferencePath(manifestPath, snapshotPath string, reference ManifestReference, digest, sourceDigest string) error {
	expectedPath := filepath.Clean(filepath.Join(filepath.Dir(manifestPath), filepath.FromSlash(reference.Path)))
	actualPath, err := filepath.Abs(snapshotPath)
	if err != nil {
		return err
	}
	expectedPath, err = filepath.Abs(expectedPath)
	if err != nil {
		return err
	}
	if actualPath != expectedPath {
		return fmt.Errorf("%w: manifest snapshot path mismatch", ErrIntegrityFailure)
	}
	if reference.Digest != digest || reference.SourceDigest != sourceDigest {
		return fmt.Errorf("%w: manifest snapshot digest mismatch", ErrIntegrityFailure)
	}
	return nil
}

func LiveSourceChanged(snapshot Snapshot, livePath string) (bool, string, error) {
	if livePath == "" {
		livePath = snapshot.Source.Path
	}
	data, err := os.ReadFile(livePath)
	if err == nil {
		actual := digestBytes(data)
		return snapshot.Source.Mode != "verify_config" || snapshot.Source.Digest != actual, actual, nil
	}
	if os.IsNotExist(err) {
		return snapshot.Source.Mode == "verify_config", "missing", nil
	}
	return false, "", err
}

func LoadAcceptanceCriteria(path string) ([]provider.Criterion, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read acceptance criteria: %w", err)
	}
	var wrapper struct {
		Criteria []provider.Criterion `json:"criteria"`
	}
	if err := json.Unmarshal(data, &wrapper); err == nil && wrapper.Criteria != nil {
		return normalizeCriteria(wrapper.Criteria), nil
	}
	var criteria []provider.Criterion
	if err := json.Unmarshal(data, &criteria); err != nil {
		return nil, fmt.Errorf("failed to parse acceptance criteria: %w", err)
	}
	return normalizeCriteria(criteria), nil
}

func AppendEvent(path string, event Event) error {
	if event.SchemaVersion == 0 {
		event.SchemaVersion = SchemaVersion
	}
	if event.RecordedAt.IsZero() {
		event.RecordedAt = time.Now().UTC()
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(append(data, '\n')); err != nil {
		return err
	}
	return file.Sync()
}

func digestBytes(data []byte) string {
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func validateAcceptanceCriteria(criteria []provider.Criterion) error {
	seen := make(map[string]struct{}, len(criteria))
	for index, criterion := range criteria {
		criterion.ID = strings.TrimSpace(criterion.ID)
		criterion.Description = strings.TrimSpace(criterion.Description)
		if criterion.ID == "" || criterion.Description == "" {
			return fmt.Errorf("acceptance criterion %d requires id and description", index)
		}
		if _, exists := seen[criterion.ID]; exists {
			return fmt.Errorf("duplicate acceptance criterion id %q", criterion.ID)
		}
		seen[criterion.ID] = struct{}{}
	}
	return nil
}

func validateSteps(steps []config.NamedVerifyStep) error {
	seen := make(map[string]struct{}, len(steps))
	for index, step := range steps {
		if strings.TrimSpace(step.Name) == "" || strings.TrimSpace(step.Command) == "" {
			return fmt.Errorf("verification step %d requires name and command", index)
		}
		if _, err := time.ParseDuration(step.Timeout); err != nil {
			return fmt.Errorf("verification step %q has invalid timeout: %w", step.Name, err)
		}
		if step.Kind != config.VerifyKindDeterministic && step.Kind != config.VerifyKindHeldOut {
			return fmt.Errorf("verification step %q has invalid kind %q", step.Name, step.Kind)
		}
		key := step.Kind + ":" + step.Name
		if _, exists := seen[key]; exists {
			return fmt.Errorf("duplicate verification step %q", key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func cloneCriteria(input []provider.Criterion) []provider.Criterion {
	if len(input) == 0 {
		return nil
	}
	return append([]provider.Criterion(nil), input...)
}

func normalizeCriteria(input []provider.Criterion) []provider.Criterion {
	criteria := cloneCriteria(input)
	for index := range criteria {
		criteria[index].ID = strings.TrimSpace(criteria[index].ID)
		criteria[index].Description = strings.TrimSpace(criteria[index].Description)
	}
	return criteria
}

func cloneSteps(input []config.NamedVerifyStep) []config.NamedVerifyStep {
	if len(input) == 0 {
		return nil
	}
	return append([]config.NamedVerifyStep(nil), input...)
}
