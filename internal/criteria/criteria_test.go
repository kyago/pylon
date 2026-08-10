package criteria

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestCreateBindsSnapshotToManifest(t *testing.T) {
	root := t.TempDir()
	workDir := filepath.Join(root, "repo")
	runDir := filepath.Join(root, "runtime", "run", "repos", "repo")
	if err := os.MkdirAll(filepath.Join(workDir, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(runDir, 0755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(workDir, ".pylon", "verify.yml")
	if err := os.WriteFile(configPath, []byte(`commands:
  - name: test
    command: go test ./...
held_out:
  - name: acceptance
    command: ./acceptance.sh
`), 0644); err != nil {
		t.Fatal(err)
	}
	acceptancePath := filepath.Join(root, "acceptance.json")
	if err := os.WriteFile(acceptancePath, []byte(`{"criteria":[{"id":"AC-1","description":"returns a stable result"}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(runDir, "status.json")
	if err := os.WriteFile(manifestPath, []byte(`{"repo_id":"repo","base_revision":"abc123"}`), 0644); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(runDir, "criteria.json")
	now := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	snapshot, err := Create(CreateOptions{
		RunID:          "run",
		RepoID:         "repo",
		BaseRevision:   "abc123",
		WorkDir:        workDir,
		ConfigPath:     configPath,
		AcceptancePath: acceptancePath,
		OutputPath:     outputPath,
		ManifestPath:   manifestPath,
		Now:            func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Verification) != 2 || snapshot.Verification[1].Kind != "held_out" {
		t.Fatalf("snapshot verification = %+v", snapshot.Verification)
	}
	if len(snapshot.AcceptanceCriteria) != 1 || snapshot.AcceptanceCriteria[0].ID != "AC-1" {
		t.Fatalf("snapshot acceptance criteria = %+v", snapshot.AcceptanceCriteria)
	}
	loaded, err := Load(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Digest != snapshot.Digest {
		t.Fatalf("loaded digest = %q, want %q", loaded.Digest, snapshot.Digest)
	}
	if err := ValidateManifest(manifestPath, outputPath, loaded); err != nil {
		t.Fatal(err)
	}
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatal(err)
	}
	reference := manifest["criteria"].(map[string]any)
	if reference["path"] != "criteria.json" || reference["digest"] != snapshot.Digest {
		t.Fatalf("manifest criteria reference = %#v", reference)
	}
}

func TestSnapshotAndManifestTamperingFailClosed(t *testing.T) {
	root := t.TempDir()
	workDir := filepath.Join(root, "repo")
	runDir := filepath.Join(root, "run")
	if err := os.MkdirAll(filepath.Join(workDir, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(runDir, 0755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(workDir, ".pylon", "verify.yml")
	if err := os.WriteFile(configPath, []byte("build:\n  command: true\n"), 0644); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(runDir, "status.json")
	if err := os.WriteFile(manifestPath, []byte(`{"repo_id":"repo","base_revision":"base"}`), 0644); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(runDir, "criteria.json")
	if _, err := Create(CreateOptions{
		RunID: "run", RepoID: "repo", BaseRevision: "base", WorkDir: workDir,
		ConfigPath: configPath, OutputPath: outputPath, ManifestPath: manifestPath,
	}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	snapshot.Verification[0].Command = "false"
	if err := writeJSON(outputPath, snapshot); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(outputPath); !errors.Is(err, ErrIntegrityFailure) {
		t.Fatalf("tampered snapshot error = %v", err)
	}

	snapshot.Digest, err = SnapshotDigest(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(outputPath, snapshot); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateManifest(manifestPath, outputPath, loaded); !errors.Is(err, ErrIntegrityFailure) {
		t.Fatalf("re-signed snapshot error = %v", err)
	}

	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest["base_revision"] = "changed-base"
	if err := writeJSON(manifestPath, manifest); err != nil {
		t.Fatal(err)
	}
	originalData, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(originalData, &snapshot); err != nil {
		t.Fatal(err)
	}
	snapshot.Verification[0].Command = "true"
	snapshot.Digest, err = SnapshotDigest(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	manifest["criteria"].(map[string]any)["digest"] = snapshot.Digest
	if err := writeJSON(outputPath, snapshot); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(manifestPath, manifest); err != nil {
		t.Fatal(err)
	}
	loaded, err = Load(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateManifest(manifestPath, outputPath, loaded); !errors.Is(err, ErrIntegrityFailure) {
		t.Fatalf("base revision tampering error = %v", err)
	}
}

func TestLiveSourceChanged(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "verify.yml")
	if err := os.WriteFile(configPath, []byte("build:\n  command: true\n"), 0644); err != nil {
		t.Fatal(err)
	}
	steps, skipped, source, err := ResolveVerification(root, configPath)
	if err != nil || skipped || len(steps) != 1 {
		t.Fatalf("resolved steps=%+v skipped=%v err=%v", steps, skipped, err)
	}
	snapshot := Snapshot{Source: source}
	changed, _, err := LiveSourceChanged(snapshot, configPath)
	if err != nil || changed {
		t.Fatalf("unchanged source changed=%v err=%v", changed, err)
	}
	if err := os.WriteFile(configPath, []byte("build:\n  command: false\n"), 0644); err != nil {
		t.Fatal(err)
	}
	changed, actual, err := LiveSourceChanged(snapshot, configPath)
	if err != nil || !changed || actual == source.Digest {
		t.Fatalf("changed source changed=%v actual=%q err=%v", changed, actual, err)
	}
}

func TestCreateDoesNotOverwriteExistingSnapshot(t *testing.T) {
	root := t.TempDir()
	workDir := filepath.Join(root, "repo")
	runDir := filepath.Join(root, "run")
	if err := os.MkdirAll(filepath.Join(workDir, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(runDir, 0755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(workDir, ".pylon", "verify.yml")
	if err := os.WriteFile(configPath, []byte("build:\n  command: \"true\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(runDir, "status.json")
	if err := os.WriteFile(manifestPath, []byte(`{"repo_id":"repo","base_revision":"base"}`), 0644); err != nil {
		t.Fatal(err)
	}
	options := CreateOptions{
		RunID: "run", RepoID: "repo", BaseRevision: "base", WorkDir: workDir,
		ConfigPath: configPath, OutputPath: filepath.Join(runDir, "criteria.json"), ManifestPath: manifestPath,
	}
	first, err := Create(options)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Create(options)
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest != second.Digest || !first.CreatedAt.Equal(second.CreatedAt) {
		t.Fatalf("idempotent create changed snapshot: first=%+v second=%+v", first, second)
	}
	if err := os.WriteFile(configPath, []byte("build:\n  command: \"false\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(options); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("changed criteria overwrite error = %v", err)
	}
}

func TestConcurrentCreateProducesOneSnapshotAndRepairsManifestBinding(t *testing.T) {
	root := t.TempDir()
	workDir := filepath.Join(root, "repo")
	runDir := filepath.Join(root, "run")
	if err := os.MkdirAll(filepath.Join(workDir, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(runDir, 0755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(workDir, ".pylon", "verify.yml")
	if err := os.WriteFile(configPath, []byte("build:\n  command: \"true\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(runDir, "status.json")
	if err := os.WriteFile(manifestPath, []byte(`{"repo_id":"repo","base_revision":"base"}`), 0644); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(runDir, "criteria.json")
	options := CreateOptions{
		RunID: "run", RepoID: "repo", BaseRevision: "base", WorkDir: workDir,
		ConfigPath: configPath, OutputPath: outputPath,
	}
	if _, err := Create(options); err != nil {
		t.Fatal(err)
	}

	options.ManifestPath = manifestPath
	var wait sync.WaitGroup
	errorsFound := make(chan error, 8)
	digests := make(chan string, 8)
	for range 8 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			snapshot, err := Create(options)
			if err != nil {
				errorsFound <- err
				return
			}
			digests <- snapshot.Digest
		}()
	}
	wait.Wait()
	close(errorsFound)
	close(digests)
	for err := range errorsFound {
		t.Fatalf("concurrent create failed: %v", err)
	}
	var digest string
	for current := range digests {
		if digest == "" {
			digest = current
		}
		if current != digest {
			t.Fatalf("concurrent creates returned different digests: %q and %q", digest, current)
		}
	}
	loaded, err := Load(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateManifest(manifestPath, outputPath, loaded); err != nil {
		t.Fatalf("manifest binding was not repaired: %v", err)
	}
}

func writeJSON(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}
