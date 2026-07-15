package history

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/store"
)

type commandCall struct {
	dir  string
	args []string
}

type fakeRunner struct {
	calls    []commandCall
	syncErr  error
	commitID string
	timeline string
}

func (r *fakeRunner) Run(dir string, args ...string) (string, error) {
	r.calls = append(r.calls, commandCall{dir: dir, args: append([]string(nil), args...)})
	if len(args) == 0 {
		return "", nil
	}
	switch args[0] {
	case "init":
		if err := os.WriteFile(args[1], []byte("repo"), 0600); err != nil {
			return "", err
		}
	case "open":
		for i := range args {
			if args[i] == "--workdir" && i+1 < len(args) {
				if err := os.MkdirAll(args[i+1], 0755); err != nil {
					return "", err
				}
				if err := os.WriteFile(filepath.Join(args[i+1], ".fslckout"), nil, 0600); err != nil {
					return "", err
				}
			}
		}
	case "commit":
		id := r.commitID
		if id == "" {
			id = "abcdef1234567890"
		}
		return "New_Version: " + id, nil
	case "sync":
		return "", r.syncErr
	case "timeline":
		if r.timeline != "" {
			return r.timeline, nil
		}
		return "pipe-1 timeline output", nil
	case "info":
		return "checkin details", nil
	case "diff":
		return "diff output", nil
	case "tarball":
		if err := writeTestTarball(args[2]); err != nil {
			return "", err
		}
	}
	return "", nil
}

func writeTestTarball(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	gz := gzip.NewWriter(file)
	tw := tar.NewWriter(gz)
	content := []byte("{}")
	if err := tw.WriteHeader(&tar.Header{Name: "pylon-history/manifest.json", Mode: 0644, Size: int64(len(content))}); err != nil {
		return err
	}
	if _, err := tw.Write(content); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	return file.Close()
}

func (r *fakeRunner) count(command string) int {
	n := 0
	for _, call := range r.calls {
		if len(call.args) > 0 && call.args[0] == command {
			n++
		}
	}
	return n
}

func newHistoryStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.NewStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Migrate(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestManagerInitializeCreatesDetachedFossilCheckout(t *testing.T) {
	root := t.TempDir()
	runner := &fakeRunner{}
	m := NewManager(root, config.HistoryConfig{}, nil, runner)

	if err := m.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, ".pylon", "history", "pylon-history.fossil")); err != nil {
		t.Fatalf("repository missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".pylon", "history", "checkout", ".fslckout")); err != nil {
		t.Fatalf("checkout metadata missing: %v", err)
	}
	if runner.count("init") != 1 || runner.count("open") != 1 {
		t.Fatalf("unexpected commands: %#v", runner.calls)
	}
	for _, call := range runner.calls {
		if len(call.args) > 0 && call.args[0] == "open" && strings.Contains(strings.Join(call.args, " "), "--empty") {
			t.Fatal("new repository must open its initial check-in, not an empty checkout")
		}
	}
	if err := m.Initialize(); err != nil {
		t.Fatalf("second Initialize failed: %v", err)
	}
	if runner.count("init") != 1 || runner.count("open") != 1 {
		t.Fatal("Initialize should be idempotent")
	}
}

func TestManagerRecognizesWindowsFossilCheckoutMetadata(t *testing.T) {
	root := t.TempDir()
	historyDir := filepath.Join(root, ".pylon", "history")
	checkoutDir := filepath.Join(historyDir, "checkout")
	if err := os.MkdirAll(checkoutDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(historyDir, "pylon-history.fossil"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(checkoutDir, "_FOSSIL_"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	m := NewManager(root, config.HistoryConfig{}, nil, &fakeRunner{})
	if !m.IsInitialized() {
		t.Fatal("_FOSSIL_ checkout metadata should be recognized")
	}
}

func TestManagerReopensExistingRepositoryAtLatestCheckin(t *testing.T) {
	root := t.TempDir()
	historyDir := filepath.Join(root, ".pylon", "history")
	if err := os.MkdirAll(historyDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(historyDir, "pylon-history.fossil"), []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}
	m := NewManager(root, config.HistoryConfig{}, nil, runner)
	if err := m.Initialize(); err != nil {
		t.Fatal(err)
	}
	if runner.count("init") != 0 || runner.count("open") != 1 {
		t.Fatalf("unexpected calls: %#v", runner.calls)
	}
	for _, call := range runner.calls {
		if len(call.args) > 0 && call.args[0] == "open" && strings.Contains(strings.Join(call.args, " "), "--empty") {
			t.Fatal("existing repository must reopen the latest check-in, not an empty checkout")
		}
	}
}

func TestManagerFossilIntegrationCheckpointDedupAndLog(t *testing.T) {
	if _, err := exec.LookPath("fossil"); err != nil {
		t.Skip("fossil binary is not installed")
	}

	root := t.TempDir()
	runtimeDir := filepath.Join(root, ".pylon", "runtime", "pipe-integration")
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtimeDir, "tasks.json"), []byte(`{"tasks":[]}`), 0644); err != nil {
		t.Fatal(err)
	}

	m := NewManager(root, config.HistoryConfig{}, nil, nil)
	if err := m.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	first, err := m.Checkpoint("pipe-integration", PhasePlanned)
	if err != nil {
		t.Fatalf("first Checkpoint failed: %v", err)
	}
	second, err := m.Checkpoint("pipe-integration", PhasePlanned)
	if err != nil {
		t.Fatalf("duplicate Checkpoint failed: %v", err)
	}
	if first.Checkin == "" || !second.Duplicate || second.Checkin != first.Checkin {
		t.Fatalf("unexpected checkpoint results: first=%#v second=%#v", first, second)
	}
	log, err := m.Log("pipe-integration", 5)
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}
	if !strings.Contains(log, "파이프라인 pipe-integration: 계획 완료") {
		t.Fatalf("checkpoint missing from Log output: %q", log)
	}
}

func TestCheckpointStoresOnlyCuratedHistory(t *testing.T) {
	root := t.TempDir()
	pipelineDir := filepath.Join(root, ".pylon", "runtime", "pipe-1")
	if err := os.MkdirAll(pipelineDir, 0755); err != nil {
		t.Fatal(err)
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(pipelineDir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("requirement.md", "로그인 구현")
	write("architecture.md", "service-a 변경")
	write("tasks.json", `{"tasks":[{"id":"T1","title":"API","description":"desc","agent":"backend","repo":"services/service-a","status":"success"}]}`)
	write("execution-log.json", `{"tasks":[{"id":"T1","agent":"backend","repo":"services/service-a","status":"success","changed_files":["api.go"],"prompt":"secret prompt","stdout":"secret output"}]}`)
	write("verification.json", `{"ok":true,"timestamp":"2026-07-14T00:00:00Z","checks":[{"name":"test","ok":true,"output":"token=secret"}]}`)
	write("pr.json", `{"url":"https://example/pr/1","number":"1","title":"로그인","body":"secret body"}`)
	write("status.json", `{"status":"success","stage":"done","sub_pipelines":[{"repo":"services/service-a","status":"success","branch":"task-login"}]}`)
	write("raw-conversation.json", `{"prompt":"must never be copied"}`)

	s := newHistoryStore(t)
	for _, entry := range []*store.MemoryEntry{
		{ProjectID: "service-a", Category: "learning", Key: "auth", Content: "JWT 사용", Author: "architect", Confidence: 0.8},
		{ProjectID: "service-a", Category: "change", Key: "api.go", Content: "raw diff", Author: "hook", Confidence: 0.7},
	} {
		if err := s.InsertMemory(entry); err != nil {
			t.Fatal(err)
		}
	}

	runner := &fakeRunner{}
	m := NewManager(root, config.HistoryConfig{}, s, runner)
	m.Now = func() time.Time { return time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC) }
	for _, phase := range []Phase{PhasePlanned, PhaseExecuted, PhaseCompleted} {
		if _, err := m.Checkpoint("pipe-1", phase); err != nil {
			t.Fatalf("Checkpoint(%s) failed: %v", phase, err)
		}
	}

	historyDir := filepath.Join(root, ".pylon", "history", "checkout", "pipelines", "pipe-1")
	execution, err := os.ReadFile(filepath.Join(historyDir, "execution-summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(execution), "secret") || !strings.Contains(string(execution), "api.go") {
		t.Fatalf("execution summary was not curated: %s", execution)
	}
	verification, err := os.ReadFile(filepath.Join(historyDir, "verification-summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(verification), "secret") || !strings.Contains(string(verification), `"ok": true`) {
		t.Fatalf("verification summary was not curated: %s", verification)
	}
	memory, err := os.ReadFile(filepath.Join(historyDir, "memory.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(memory), "raw diff") || !strings.Contains(string(memory), "JWT 사용") {
		t.Fatalf("memory export was not curated: %s", memory)
	}

	var manifest Manifest
	manifestBytes, err := os.ReadFile(filepath.Join(historyDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.PipelineID != "pipe-1" || manifest.Phase != PhaseCompleted || len(manifest.Artifacts) == 0 {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}
	if len(manifest.AffectedProjects) != 1 || manifest.AffectedProjects[0] != "services/service-a" {
		t.Fatalf("affected projects = %#v", manifest.AffectedProjects)
	}
	if runner.count("commit") != 3 {
		t.Fatalf("commit count = %d", runner.count("commit"))
	}
	if _, err := os.Stat(filepath.Join(historyDir, "raw-conversation.json")); !os.IsNotExist(err) {
		t.Fatal("raw conversation must not be copied into history")
	}
}

func TestCheckpointDeduplicatesSameContent(t *testing.T) {
	root := t.TempDir()
	pipelineDir := filepath.Join(root, ".pylon", "runtime", "pipe-1")
	if err := os.MkdirAll(pipelineDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pipelineDir, "tasks.json"), []byte(`{"tasks":[]}`), 0644); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}
	m := NewManager(root, config.HistoryConfig{}, nil, runner)

	first, err := m.Checkpoint("pipe-1", PhasePlanned)
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.Checkpoint("pipe-1", PhasePlanned)
	if err != nil {
		t.Fatal(err)
	}
	if first.Duplicate || !second.Duplicate || first.Checkin != second.Checkin {
		t.Fatalf("unexpected results: first=%#v second=%#v", first, second)
	}
	if runner.count("commit") != 1 {
		t.Fatalf("commit count = %d", runner.count("commit"))
	}
	manifestBytes, err := os.ReadFile(filepath.Join(root, ".pylon", "history", "checkout", "pipelines", "pipe-1", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Status != "planned" {
		t.Fatalf("manifest status = %q", manifest.Status)
	}
}

func TestCheckpointKeepsLocalCommitWhenSyncFails(t *testing.T) {
	root := t.TempDir()
	pipelineDir := filepath.Join(root, ".pylon", "runtime", "pipe-1")
	if err := os.MkdirAll(pipelineDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pipelineDir, "tasks.json"), []byte(`{"tasks":[]}`), 0644); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{syncErr: errors.New("offline")}
	m := NewManager(root, config.HistoryConfig{Remote: "https://history.example.com/pylon", SyncOnCheckpoint: true}, nil, runner)

	result, err := m.Checkpoint("pipe-1", PhasePlanned)
	if err != nil {
		t.Fatalf("local checkpoint should succeed: %v", err)
	}
	if !result.PendingSync || runner.count("sync") != 1 {
		t.Fatalf("result=%#v calls=%#v", result, runner.calls)
	}
}

func TestCheckpointMarksPendingWhenAutomaticSyncDisabled(t *testing.T) {
	root := t.TempDir()
	pipelineDir := filepath.Join(root, ".pylon", "runtime", "pipe-1")
	if err := os.MkdirAll(pipelineDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pipelineDir, "tasks.json"), []byte(`{"tasks":[]}`), 0644); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}
	m := NewManager(root, config.HistoryConfig{Remote: "https://history.example.com/pylon", SyncOnCheckpoint: false}, nil, runner)

	result, err := m.Checkpoint("pipe-1", PhasePlanned)
	if err != nil {
		t.Fatal(err)
	}
	if !result.PendingSync || runner.count("sync") != 0 {
		t.Fatalf("disabled automatic sync must remain pending: result=%#v calls=%#v", result, runner.calls)
	}
}

func TestCompletedCheckpointAggregatesSubpipelineResults(t *testing.T) {
	root := t.TempDir()
	pipelineDir := filepath.Join(root, ".pylon", "runtime", "pipe-1")
	subDir := filepath.Join(pipelineDir, "service-a")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join(pipelineDir, "tasks.json"):   `{"tasks":[{"id":"T1","repo":"service-a"}]}`,
		filepath.Join(pipelineDir, "status.json"):  `{"status":"success","sub_pipelines":[{"repo":"service-a","pipeline_dir":"service-a","status":"success"}]}`,
		filepath.Join(subDir, "verification.json"): `{"ok":true,"checks":[{"name":"test","ok":true,"output":"secret"}]}`,
		filepath.Join(subDir, "pr.json"):           `{"url":"https://example/pr/1","number":"1","title":"API","body":"secret"}`,
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	m := NewManager(root, config.HistoryConfig{}, nil, &fakeRunner{})
	if _, err := m.Checkpoint("pipe-1", PhaseCompleted); err != nil {
		t.Fatal(err)
	}
	historyDir := filepath.Join(root, ".pylon", "history", "checkout", "pipelines", "pipe-1")
	for _, name := range []string{"verification-summary.json", "pr-summary.json"} {
		data, err := os.ReadFile(filepath.Join(historyDir, name))
		if err != nil {
			t.Fatalf("%s missing: %v", name, err)
		}
		if !strings.Contains(string(data), "service-a") || strings.Contains(string(data), "secret") {
			t.Fatalf("%s was not safely aggregated: %s", name, data)
		}
	}
}

func TestExecutedCheckpointFallsBackToSubpipelineStatus(t *testing.T) {
	root := t.TempDir()
	pipelineDir := filepath.Join(root, ".pylon", "runtime", "pipe-1")
	if err := os.MkdirAll(pipelineDir, 0755); err != nil {
		t.Fatal(err)
	}
	status := `{"status":"running","sub_pipelines":[{"repo":"service-a","pipeline_dir":"service-a","status":"failed","error":"build failed"}]}`
	if err := os.WriteFile(filepath.Join(pipelineDir, "status.json"), []byte(status), 0644); err != nil {
		t.Fatal(err)
	}
	m := NewManager(root, config.HistoryConfig{}, nil, &fakeRunner{})
	if _, err := m.Checkpoint("pipe-1", PhaseExecuted); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".pylon", "history", "checkout", "pipelines", "pipe-1", "execution-summary.json"))
	if err != nil {
		t.Fatalf("execution fallback missing: %v", err)
	}
	if !strings.Contains(string(data), "service-a") || !strings.Contains(string(data), "build failed") {
		t.Fatalf("unexpected execution fallback: %s", data)
	}
}

func TestConcurrentCheckpointUsesSingleWriter(t *testing.T) {
	root := t.TempDir()
	pipelineDir := filepath.Join(root, ".pylon", "runtime", "pipe-1")
	if err := os.MkdirAll(pipelineDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pipelineDir, "tasks.json"), []byte(`{"tasks":[]}`), 0644); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}
	m := NewManager(root, config.HistoryConfig{}, nil, runner)

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := m.Checkpoint("pipe-1", PhasePlanned)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if runner.count("commit") != 1 {
		t.Fatalf("concurrent duplicate created %d commits", runner.count("commit"))
	}
}

func TestAcquireLockTimeoutReportsRecoveryPath(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), ".checkpoint.lock")
	if err := os.Mkdir(lockPath, 0700); err != nil {
		t.Fatal(err)
	}

	_, err := acquireLock(lockPath, 20*time.Millisecond)
	if err == nil {
		t.Fatal("expected lock timeout")
	}
	message := err.Error()
	if !strings.Contains(message, lockPath) || !strings.Contains(message, "제거") {
		t.Fatalf("lock timeout must include path and recovery guidance: %q", message)
	}
}

func TestHistoryReadAndSyncCommandsUseDetachedRepository(t *testing.T) {
	root := t.TempDir()
	runner := &fakeRunner{timeline: "12:00:01 [abc] 파이프라인 pipe-1: 계획 완료 (user: test)"}
	m := NewManager(root, config.HistoryConfig{Remote: "https://history.example.com/pylon"}, nil, runner)
	if err := m.Initialize(); err != nil {
		t.Fatal(err)
	}

	if output, err := m.Log("pipe-1", 5); err != nil || output != runner.timeline {
		t.Fatalf("Log output=%q err=%v", output, err)
	}
	if output, err := m.Show("abc123"); err != nil || output != "checkin details" {
		t.Fatalf("Show output=%q err=%v", output, err)
	}
	if output, err := m.Diff("abc123", "def456"); err != nil || output != "diff output" {
		t.Fatalf("Diff output=%q err=%v", output, err)
	}
	if err := m.Sync(); err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	wantRepo := filepath.Join(root, ".pylon", "history", "pylon-history.fossil")
	for _, command := range []string{"timeline", "info", "diff", "sync"} {
		foundRepo := false
		for _, call := range runner.calls {
			if len(call.args) > 0 && call.args[0] == command && strings.Contains(strings.Join(call.args, " "), wantRepo) {
				foundRepo = true
			}
		}
		if !foundRepo {
			t.Fatalf("%s did not target detached repository: %#v", command, runner.calls)
		}
	}
}

func TestHistoryLogFiltersExactPipelineAndRemovesFooter(t *testing.T) {
	root := t.TempDir()
	runner := &fakeRunner{timeline: strings.Join([]string{
		"12:00:02 [bbb] 파이프라인 pipe-10: 계획 완료 (user: test)",
		"12:00:01 [aaa] 파이프라인 pipe-1: 계획 완료 (user: test)",
		"+++ no more data (2) +++",
	}, "\n")}
	m := NewManager(root, config.HistoryConfig{}, nil, runner)
	if err := m.Initialize(); err != nil {
		t.Fatal(err)
	}

	filtered, err := m.Log("pipe-1", 5)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(filtered, "pipe-10") || !strings.Contains(filtered, "파이프라인 pipe-1:") {
		t.Fatalf("pipeline filter used a prefix match: %q", filtered)
	}
	all, err := m.Log("", 5)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(all, "+++ no more data") {
		t.Fatalf("Fossil footer leaked into log output: %q", all)
	}
}

func TestHistoryExportRefusesExistingDestination(t *testing.T) {
	root := t.TempDir()
	runner := &fakeRunner{}
	m := NewManager(root, config.HistoryConfig{}, nil, runner)
	if err := m.Initialize(); err != nil {
		t.Fatal(err)
	}

	output := filepath.Join(root, "exported")
	if err := m.Export("abc123", output); err != nil {
		t.Fatalf("Export failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(output, "manifest.json")); err != nil {
		t.Fatalf("exported content missing: %v", err)
	}
	if err := m.Export("abc123", output); err == nil {
		t.Fatal("expected existing output directory to be rejected")
	}
}

func TestExtractTarballRejectsPathTraversal(t *testing.T) {
	root := t.TempDir()
	archive := filepath.Join(root, "unsafe.tar.gz")
	file, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(file)
	tw := tar.NewWriter(gz)
	content := []byte("escape")
	if err := tw.WriteHeader(&tar.Header{Name: "pylon-history/../../escape.txt", Mode: 0644, Size: int64(len(content))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	if err := extractTarball(archive, filepath.Join(root, "output")); err == nil {
		t.Fatal("expected unsafe archive path to be rejected")
	}
	if _, err := os.Stat(filepath.Join(root, "escape.txt")); !os.IsNotExist(err) {
		t.Fatal("unsafe archive escaped the output directory")
	}
}

func TestVerifyFossilReportsVersionFromPath(t *testing.T) {
	binDir := t.TempDir()
	fossilPath := filepath.Join(binDir, "fossil")
	if err := os.WriteFile(fossilPath, []byte("#!/bin/sh\necho 'This is fossil version 2.28 [abc]'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	version, err := VerifyFossil()
	if err != nil {
		t.Fatalf("VerifyFossil failed: %v", err)
	}
	if version != "2.28" {
		t.Fatalf("version = %q", version)
	}
}

func TestVerifyFossilFailsWhenMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := VerifyFossil(); err == nil {
		t.Fatal("expected missing fossil error")
	}
}
