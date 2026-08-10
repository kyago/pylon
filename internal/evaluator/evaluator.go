// Package evaluator prepares isolated held-out verification bundles.
package evaluator

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kyago/pylon/internal/criteria"
	"github.com/kyago/pylon/internal/fsutil"
	"github.com/kyago/pylon/internal/provider"
)

const SchemaVersion = 1

var (
	ErrInvalidInput      = errors.New("invalid evaluator input")
	ErrIntegrityFailure  = errors.New("evaluator bundle integrity failure")
	ErrInsufficientGate  = errors.New("deterministic verification has not passed")
	ErrUnsafeOutputPath  = errors.New("evaluator bundle must be outside the implementation root")
	ErrPolicyUnsupported = errors.New("provider cannot enforce evaluator policy")
)

type InputFile struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
}

type Policy struct {
	AccessMode           string                 `json:"access_mode"`
	InputPolicy          string                 `json:"input_policy"`
	RequiredCapabilities provider.CapabilitySet `json:"required_capabilities"`
	DeniedCapabilities   []provider.Capability  `json:"denied_capabilities"`
}

type Request struct {
	SchemaVersion  int         `json:"schema_version"`
	RunID          string      `json:"run_id"`
	TaskID         string      `json:"task_id"`
	RepoID         string      `json:"repo_id"`
	CriteriaDigest string      `json:"criteria_digest"`
	CriteriaIDs    []string    `json:"criteria_ids"`
	Inputs         []InputFile `json:"inputs"`
	Policy         Policy      `json:"policy"`
	CreatedAt      time.Time   `json:"created_at"`
	Digest         string      `json:"digest"`
}

type PrepareOptions struct {
	TaskID             string
	ImplementationRoot string
	RequirementPath    string
	CriteriaPath       string
	VerificationPath   string
	DiffPath           string
	TaskReportPath     string
	OutputDir          string
	ManifestPath       string
	Now                func() time.Time
}

type RecordOptions struct {
	BundleDir    string
	ManifestPath string
	InputPath    string
	OutputPath   string
	Now          func() time.Time
}

type ManifestReference struct {
	Path      string    `json:"path"`
	Digest    string    `json:"digest"`
	CreatedAt time.Time `json:"created_at"`
}

type VerdictStatus string

const (
	VerdictPass       VerdictStatus = "pass"
	VerdictFail       VerdictStatus = "fail"
	VerdictIncomplete VerdictStatus = "incomplete"
)

type CriterionVerdict struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Evidence string `json:"evidence"`
}

type Verdict struct {
	SchemaVersion int                `json:"schema_version"`
	RequestDigest string             `json:"request_digest"`
	Status        VerdictStatus      `json:"status"`
	Summary       string             `json:"summary"`
	Criteria      []CriterionVerdict `json:"criteria"`
	Risks         []string           `json:"risks,omitempty"`
	EvidenceRefs  []string           `json:"evidence_refs,omitempty"`
	Evaluator     string             `json:"evaluator"`
	RecordedAt    time.Time          `json:"recorded_at"`
}

type deterministicResult struct {
	OK             bool   `json:"ok"`
	CriteriaDigest string `json:"criteria_digest"`
}

func DefaultPolicy() Policy {
	return Policy{
		AccessMode:  "read_only",
		InputPolicy: "isolated_evidence",
		RequiredCapabilities: provider.CapabilitySet{
			ReadFiles:        true,
			StructuredOutput: true,
			ToolRestrictions: true,
		},
		DeniedCapabilities: []provider.Capability{
			provider.CapabilityEditFiles,
			provider.CapabilityRunShell,
			provider.CapabilitySpawnSubagents,
			provider.CapabilityBackgroundExecution,
		},
	}
}

func (policy Policy) ValidateProvider(capabilities provider.CapabilitySet) error {
	missing := capabilities.Missing(policy.RequiredCapabilities)
	if len(missing) > 0 {
		return fmt.Errorf("%w: missing %v", ErrPolicyUnsupported, missing)
	}
	return nil
}

func Prepare(options PrepareOptions) (Request, error) {
	if strings.TrimSpace(options.TaskID) == "" {
		return Request{}, fmt.Errorf("%w: task id is required", ErrInvalidInput)
	}
	if options.OutputDir == "" || options.ImplementationRoot == "" || options.ManifestPath == "" {
		return Request{}, fmt.Errorf("%w: implementation root, output directory, and manifest are required", ErrInvalidInput)
	}
	inside, err := isWithin(options.ImplementationRoot, options.OutputDir)
	if err != nil {
		return Request{}, err
	}
	if inside {
		return Request{}, ErrUnsafeOutputPath
	}
	snapshot, err := criteria.Load(options.CriteriaPath)
	if err != nil {
		return Request{}, err
	}
	verification, err := loadDeterministicResult(options.VerificationPath)
	if err != nil {
		return Request{}, err
	}
	if !verification.OK || verification.CriteriaDigest != snapshot.Digest {
		return Request{}, fmt.Errorf("%w: verification digest %q, criteria digest %q", ErrInsufficientGate, verification.CriteriaDigest, snapshot.Digest)
	}
	if len(snapshot.AcceptanceCriteria) == 0 {
		return Request{}, fmt.Errorf("%w: criteria snapshot has no acceptance criteria", ErrInvalidInput)
	}
	inputs := []struct {
		name string
		path string
	}{
		{name: "requirement.md", path: options.RequirementPath},
		{name: "criteria.json", path: options.CriteriaPath},
		{name: "deterministic-verification.json", path: options.VerificationPath},
		{name: "change.diff", path: options.DiffPath},
	}
	if options.TaskReportPath != "" {
		inputs = append(inputs, struct {
			name string
			path string
		}{name: "task-report.json", path: options.TaskReportPath})
	}
	for _, input := range inputs {
		if err := validateInputPath(input.path); err != nil {
			return Request{}, err
		}
	}
	if _, err := os.Stat(options.OutputDir); err == nil {
		return Request{}, fmt.Errorf("%w: output already exists", ErrInvalidInput)
	} else if !os.IsNotExist(err) {
		return Request{}, err
	}
	if err := os.MkdirAll(filepath.Dir(options.OutputDir), 0755); err != nil {
		return Request{}, err
	}
	tempDir, err := os.MkdirTemp(filepath.Dir(options.OutputDir), ".evaluator-")
	if err != nil {
		return Request{}, err
	}
	defer os.RemoveAll(tempDir)

	requestInputs := make([]InputFile, 0, len(inputs))
	for _, input := range inputs {
		target := filepath.Join(tempDir, input.name)
		file, err := copyInput(input.path, target)
		if err != nil {
			return Request{}, err
		}
		requestInputs = append(requestInputs, file)
	}
	sort.Slice(requestInputs, func(i, j int) bool { return requestInputs[i].Name < requestInputs[j].Name })
	now := time.Now
	if options.Now != nil {
		now = options.Now
	}
	request := Request{
		SchemaVersion:  SchemaVersion,
		RunID:          snapshot.RunID,
		TaskID:         options.TaskID,
		RepoID:         snapshot.RepoID,
		CriteriaDigest: snapshot.Digest,
		CriteriaIDs:    make([]string, 0, len(snapshot.AcceptanceCriteria)),
		Inputs:         requestInputs,
		Policy:         DefaultPolicy(),
		CreatedAt:      now().UTC(),
	}
	for _, criterion := range snapshot.AcceptanceCriteria {
		request.CriteriaIDs = append(request.CriteriaIDs, criterion.ID)
	}
	request.Digest, err = RequestDigest(request)
	if err != nil {
		return Request{}, err
	}
	if err := fsutil.WriteJSONAtomic(filepath.Join(tempDir, "request.json"), request); err != nil {
		return Request{}, err
	}
	if err := makeReadOnly(tempDir); err != nil {
		return Request{}, err
	}
	if err := os.Rename(tempDir, options.OutputDir); err != nil {
		return Request{}, err
	}
	if err := BindManifest(options.ManifestPath, options.OutputDir, request); err != nil {
		_ = makeWritable(options.OutputDir)
		_ = os.RemoveAll(options.OutputDir)
		return Request{}, err
	}
	return request, nil
}

func Load(bundleDir string) (Request, error) {
	requestPath := filepath.Join(bundleDir, "request.json")
	bundleInfo, err := os.Stat(bundleDir)
	if err != nil {
		return Request{}, err
	}
	if !bundleInfo.IsDir() || bundleInfo.Mode().Perm()&0222 != 0 {
		return Request{}, fmt.Errorf("%w: bundle directory is not read-only", ErrIntegrityFailure)
	}
	requestInfo, err := os.Stat(requestPath)
	if err != nil {
		return Request{}, err
	}
	if !requestInfo.Mode().IsRegular() || requestInfo.Mode().Perm()&0222 != 0 {
		return Request{}, fmt.Errorf("%w: request manifest is not read-only", ErrIntegrityFailure)
	}
	data, err := os.ReadFile(requestPath)
	if err != nil {
		return Request{}, err
	}
	var request Request
	if err := json.Unmarshal(data, &request); err != nil {
		return Request{}, err
	}
	if request.SchemaVersion != SchemaVersion {
		return Request{}, fmt.Errorf("%w: unsupported schema version %d", ErrIntegrityFailure, request.SchemaVersion)
	}
	digest, err := RequestDigest(request)
	if err != nil {
		return Request{}, err
	}
	if request.Digest == "" || request.Digest != digest {
		return Request{}, fmt.Errorf("%w: request digest mismatch", ErrIntegrityFailure)
	}
	if request.Policy.AccessMode != "read_only" || request.Policy.InputPolicy != "isolated_evidence" {
		return Request{}, fmt.Errorf("%w: unsafe evaluator policy", ErrIntegrityFailure)
	}
	if len(request.CriteriaIDs) == 0 {
		return Request{}, fmt.Errorf("%w: request has no acceptance criteria", ErrIntegrityFailure)
	}
	allowed := map[string]bool{
		"requirement.md":                  true,
		"criteria.json":                   true,
		"deterministic-verification.json": true,
		"change.diff":                     true,
		"task-report.json":                true,
	}
	for _, input := range request.Inputs {
		if !allowed[input.Name] || filepath.Base(input.Path) != input.Name {
			return Request{}, fmt.Errorf("%w: unexpected input %q", ErrIntegrityFailure, input.Name)
		}
		path := filepath.Join(bundleDir, input.Path)
		info, err := os.Lstat(path)
		if err != nil {
			return Request{}, err
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0222 != 0 {
			return Request{}, fmt.Errorf("%w: input %s is not read-only", ErrIntegrityFailure, input.Name)
		}
		digest, size, err := fileDigest(path)
		if err != nil {
			return Request{}, err
		}
		if digest != input.Digest || size != input.Size {
			return Request{}, fmt.Errorf("%w: input %s changed", ErrIntegrityFailure, input.Name)
		}
	}
	return request, nil
}

func LoadBound(bundleDir, manifestPath string) (Request, error) {
	request, err := Load(bundleDir)
	if err != nil {
		return Request{}, err
	}
	if err := ValidateManifest(manifestPath, bundleDir, request); err != nil {
		return Request{}, err
	}
	return request, nil
}

func BindManifest(manifestPath, bundleDir string, request Request) error {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		return err
	}
	requestPath := filepath.Join(bundleDir, "request.json")
	relative, err := filepath.Rel(filepath.Dir(manifestPath), requestPath)
	if err != nil {
		return err
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("evaluator bundle must be inside the run manifest directory")
	}
	reference := ManifestReference{Path: filepath.ToSlash(relative), Digest: request.Digest, CreatedAt: request.CreatedAt}
	if raw, exists := manifest["evaluator_request"]; exists && raw != nil {
		existingData, marshalErr := json.Marshal(raw)
		if marshalErr != nil {
			return marshalErr
		}
		var existing ManifestReference
		if unmarshalErr := json.Unmarshal(existingData, &existing); unmarshalErr != nil {
			return unmarshalErr
		}
		if existing != reference {
			return errors.New("run manifest already references a different evaluator request")
		}
		return nil
	}
	manifest["evaluator_request"] = reference
	return fsutil.WriteJSONAtomic(manifestPath, manifest)
}

func ValidateManifest(manifestPath, bundleDir string, request Request) error {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrIntegrityFailure, err)
	}
	var manifest struct {
		EvaluatorRequest *ManifestReference `json:"evaluator_request"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("%w: %v", ErrIntegrityFailure, err)
	}
	if manifest.EvaluatorRequest == nil {
		return fmt.Errorf("%w: manifest has no evaluator request", ErrIntegrityFailure)
	}
	expectedPath, err := filepath.Abs(filepath.Join(filepath.Dir(manifestPath), filepath.FromSlash(manifest.EvaluatorRequest.Path)))
	if err != nil {
		return err
	}
	actualPath, err := filepath.Abs(filepath.Join(bundleDir, "request.json"))
	if err != nil {
		return err
	}
	if expectedPath != actualPath || manifest.EvaluatorRequest.Digest != request.Digest || !manifest.EvaluatorRequest.CreatedAt.Equal(request.CreatedAt) {
		return fmt.Errorf("%w: evaluator request manifest mismatch", ErrIntegrityFailure)
	}
	return nil
}

func RequestDigest(request Request) (string, error) {
	request.Digest = ""
	data, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	return digestBytes(data), nil
}

func ValidateVerdict(request Request, verdict Verdict) error {
	if verdict.SchemaVersion != SchemaVersion {
		return fmt.Errorf("invalid verdict schema version %d", verdict.SchemaVersion)
	}
	if verdict.RequestDigest != request.Digest {
		return errors.New("verdict request digest mismatch")
	}
	switch verdict.Status {
	case VerdictPass, VerdictFail, VerdictIncomplete:
	default:
		return fmt.Errorf("invalid verdict status %q", verdict.Status)
	}
	if strings.TrimSpace(verdict.Summary) == "" || strings.TrimSpace(verdict.Evaluator) == "" {
		return errors.New("verdict summary and evaluator are required")
	}
	if len(verdict.Criteria) == 0 {
		return errors.New("verdict must include criterion coverage")
	}
	expected := make(map[string]struct{}, len(request.CriteriaIDs))
	for _, id := range request.CriteriaIDs {
		expected[id] = struct{}{}
	}
	seen := make(map[string]struct{}, len(verdict.Criteria))
	allVerified := true
	for _, criterion := range verdict.Criteria {
		if _, ok := expected[criterion.ID]; !ok {
			return fmt.Errorf("verdict contains unknown criterion %q", criterion.ID)
		}
		if _, duplicate := seen[criterion.ID]; duplicate {
			return fmt.Errorf("verdict contains duplicate criterion %q", criterion.ID)
		}
		seen[criterion.ID] = struct{}{}
		switch criterion.Status {
		case "verified":
		case "partial", "missing":
			allVerified = false
		default:
			return fmt.Errorf("criterion %q has invalid status %q", criterion.ID, criterion.Status)
		}
		if strings.TrimSpace(criterion.Evidence) == "" {
			return fmt.Errorf("criterion %q has no evidence", criterion.ID)
		}
	}
	if len(seen) != len(expected) {
		return errors.New("verdict does not cover every acceptance criterion")
	}
	if verdict.Status == VerdictPass && !allVerified {
		return errors.New("pass verdict requires every criterion to be verified")
	}
	allowedEvidence := make(map[string]struct{}, len(request.Inputs))
	for _, input := range request.Inputs {
		allowedEvidence[input.Name] = struct{}{}
	}
	for _, reference := range verdict.EvidenceRefs {
		if _, ok := allowedEvidence[reference]; !ok {
			return fmt.Errorf("verdict references evidence outside the bundle: %q", reference)
		}
	}
	return nil
}

func Record(options RecordOptions) (Verdict, error) {
	if options.BundleDir == "" || options.ManifestPath == "" || options.InputPath == "" || options.OutputPath == "" {
		return Verdict{}, errors.New("bundle, manifest, input, and output are required")
	}
	request, err := LoadBound(options.BundleDir, options.ManifestPath)
	if err != nil {
		return Verdict{}, err
	}
	data, err := os.ReadFile(options.InputPath)
	if err != nil {
		return Verdict{}, err
	}
	var verdict Verdict
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&verdict); err != nil {
		return Verdict{}, err
	}
	manifestDir, err := filepath.Abs(filepath.Dir(options.ManifestPath))
	if err != nil {
		return Verdict{}, err
	}
	outputPath, err := filepath.Abs(options.OutputPath)
	if err != nil {
		return Verdict{}, err
	}
	insideManifest, err := isWithin(manifestDir, outputPath)
	if err != nil {
		return Verdict{}, err
	}
	insideBundle, err := isWithin(options.BundleDir, outputPath)
	if err != nil {
		return Verdict{}, err
	}
	if !insideManifest || insideBundle {
		return Verdict{}, errors.New("evaluator result must be inside the run directory and outside the read-only bundle")
	}
	if verdict.RecordedAt.IsZero() {
		now := time.Now
		if options.Now != nil {
			now = options.Now
		}
		verdict.RecordedAt = now().UTC()
	}
	if err := ValidateVerdict(request, verdict); err != nil {
		return Verdict{}, err
	}
	if err := fsutil.WriteJSONAtomic(options.OutputPath, verdict); err != nil {
		return Verdict{}, err
	}
	return verdict, nil
}

func loadDeterministicResult(path string) (deterministicResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return deterministicResult{}, err
	}
	var result deterministicResult
	if err := json.Unmarshal(data, &result); err != nil {
		return deterministicResult{}, err
	}
	return result, nil
}

func validateInputPath(path string) error {
	if path == "" {
		return fmt.Errorf("%w: required input path is empty", ErrInvalidInput)
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	for _, forbidden := range []string{"/.pylon/memory/", "/.pylon/conversations/", "/transcripts/", "/conversation"} {
		if strings.Contains("/"+strings.TrimPrefix(clean, "/"), forbidden) {
			return fmt.Errorf("%w: forbidden evaluator context %s", ErrInvalidInput, path)
		}
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: input must be a regular file: %s", ErrInvalidInput, path)
	}
	return nil
}

func copyInput(source, target string) (InputFile, error) {
	input, err := os.Open(source)
	if err != nil {
		return InputFile{}, err
	}
	defer input.Close()
	output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0444)
	if err != nil {
		return InputFile{}, err
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(output, hash), input)
	closeErr := output.Close()
	if copyErr != nil {
		return InputFile{}, copyErr
	}
	if closeErr != nil {
		return InputFile{}, closeErr
	}
	if err := os.Chmod(target, 0444); err != nil {
		return InputFile{}, err
	}
	return InputFile{
		Name:   filepath.Base(target),
		Path:   filepath.Base(target),
		Digest: "sha256:" + hex.EncodeToString(hash.Sum(nil)),
		Size:   written,
	}, nil
}

func makeReadOnly(root string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.Chmod(path, 0555)
		}
		return os.Chmod(path, 0444)
	})
}

func fileDigest(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return "", 0, err
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), size, nil
}

func digestBytes(data []byte) string {
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func isWithin(root, target string) (bool, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false, err
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return false, err
	}
	relative, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil {
		return false, err
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))), nil
}

func makeWritable(root string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.Chmod(path, 0755)
		}
		return os.Chmod(path, 0644)
	})
}
