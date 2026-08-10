// internal/history/manager_test.go
package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// mustWrite writes a setup file, failing the test on error.
func mustWrite(t *testing.T, path string, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
}

// mustCheckpoint records a checkpoint in setup, failing the test on error.
func mustCheckpoint(t *testing.T, m *Manager, pipelineID string, phase Phase) CheckpointResult {
	t.Helper()
	result, err := m.Checkpoint(pipelineID, phase)
	if err != nil {
		t.Fatalf("Checkpoint(%s, %s) 실패: %v", pipelineID, phase, err)
	}
	return result
}

// newTestManager는 runtime 디렉토리에 최소 파이프라인 산출물을 깔아 둔 Manager를 만든다.
func newTestManager(t *testing.T) (*Manager, string) {
	t.Helper()
	root := t.TempDir()
	pipelineDir := filepath.Join(root, ".pylon", "runtime", "pipe-1")
	if err := os.MkdirAll(pipelineDir, 0755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(pipelineDir, "requirement.md"), "# 요구사항")
	mustWrite(t, filepath.Join(pipelineDir, "tasks.json"), `{"tasks":[{"id":"T1","repo":"app","secret_field":"drop-me"}]}`)
	mustWrite(t, filepath.Join(pipelineDir, "status.json"), `{"status":"completed","stage":"done","noise":"drop-me"}`)
	m := NewManager(root)
	m.Now = func() time.Time { return time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC) }
	return m, root
}

func TestCheckpointCreatesSnapshotDirectory(t *testing.T) {
	m, root := newTestManager(t)
	result, err := m.Checkpoint("pipe-1", PhasePlanned)
	if err != nil {
		t.Fatalf("Checkpoint 실패: %v", err)
	}
	if result.Ref != "pipe-1/planned" || result.Duplicate {
		t.Fatalf("결과 불일치: %+v", result)
	}
	snapDir := filepath.Join(root, ".pylon", "history", "pipelines", "pipe-1", "planned")
	data, err := os.ReadFile(filepath.Join(snapDir, "manifest.json"))
	if err != nil {
		t.Fatalf("manifest.json이 있어야 한다: %v", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Digest != result.Digest || manifest.PipelineID != "pipe-1" {
		t.Fatalf("manifest 불일치: %+v vs %+v", manifest, result)
	}
	if _, err := os.Stat(filepath.Join(snapDir, "requirement.md")); err != nil {
		t.Error("requirement.md가 스냅샷에 있어야 한다")
	}
	if manifest.AffectedProjects[0] != "app" {
		t.Errorf("affected_projects: %v", manifest.AffectedProjects)
	}
}

func TestCheckpointDeduplicatesByDigest(t *testing.T) {
	m, _ := newTestManager(t)
	first, err := m.Checkpoint("pipe-1", PhasePlanned)
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.Checkpoint("pipe-1", PhasePlanned)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Duplicate || second.Digest != first.Digest {
		t.Fatalf("동일 내용 재체크포인트는 Duplicate여야 한다: %+v", second)
	}
}

func TestValidateDetectsCheckpointTampering(t *testing.T) {
	m, root := newTestManager(t)
	mustCheckpoint(t, m, "pipe-1", PhaseCompleted)
	if _, err := m.Validate("pipe-1/completed"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".pylon", "history", "pipelines", "pipe-1", "completed", "status-summary.json")
	mustWrite(t, path, `{"status":"tampered"}`)
	if _, err := m.Validate("pipe-1/completed"); err == nil {
		t.Fatal("tampered checkpoint validated")
	}
}

func TestTerminalPhaseCopiesMemorySnapshot(t *testing.T) {
	m, root := newTestManager(t)
	memFile := filepath.Join(root, ".pylon", "memory", "app", "learning", "k.md")
	mustWrite(t, memFile, "---\nkey: k\ncategory: learning\nconfidence: 0.8\ncreated_at: 2026-07-23T00:00:00Z\n---\n\n내용\n")

	if _, err := m.Checkpoint("pipe-1", PhaseCompleted); err != nil {
		t.Fatalf("Checkpoint 실패: %v", err)
	}
	copied := filepath.Join(root, ".pylon", "history", "pipelines", "pipe-1", "completed", "memory", "app", "learning", "k.md")
	if _, err := os.Stat(copied); err != nil {
		t.Errorf("종료 체크포인트에 메모리 스냅샷이 복사되어야 한다: %v", err)
	}
}

// 이력 스냅샷에는 큐레이션된 카테고리만 남아야 한다 — raw 변경 이력 등은 제외.
func TestTerminalPhaseExcludesUncuratedMemoryCategories(t *testing.T) {
	m, root := newTestManager(t)
	mustWrite(t, filepath.Join(root, ".pylon", "memory", "app", "learning", "keep.md"),
		"---\nkey: k\ncategory: learning\nconfidence: 0.8\ncreated_at: 2026-07-23T00:00:00Z\n---\n\n유지\n")
	mustWrite(t, filepath.Join(root, ".pylon", "memory", "app", "change", "raw.md"),
		"---\nkey: k\ncategory: change\nconfidence: 0.8\ncreated_at: 2026-07-23T00:00:00Z\n---\n\nRAW_DIFF\n")

	mustCheckpoint(t, m, "pipe-1", PhaseCompleted)

	snap := filepath.Join(root, ".pylon", "history", "pipelines", "pipe-1", "completed", "memory", "app")
	if _, err := os.Stat(filepath.Join(snap, "learning", "keep.md")); err != nil {
		t.Errorf("learning 카테고리는 보존되어야 한다: %v", err)
	}
	if _, err := os.Stat(filepath.Join(snap, "change", "raw.md")); err == nil {
		t.Error("change 카테고리는 이력에 복사되면 안 된다")
	}
}

// 서로 다른 repo가 같은 basename을 가져도 체크포인트가 실패하면 안 된다.
func TestTerminalPhaseHandlesProjectBasenameCollision(t *testing.T) {
	m, root := newTestManager(t)
	mustWrite(t, filepath.Join(root, ".pylon", "runtime", "pipe-2", "tasks.json"),
		`{"tasks":[{"repo":"services/api"},{"repo":"vendor/api"}]}`)
	mustWrite(t, filepath.Join(root, ".pylon", "runtime", "pipe-2", "status.json"), `{"status":"completed"}`)
	mustWrite(t, filepath.Join(root, ".pylon", "memory", "api", "learning", "a.md"),
		"---\nkey: k\ncategory: learning\nconfidence: 0.8\ncreated_at: 2026-07-23T00:00:00Z\n---\n\n본문\n")

	if _, err := m.Checkpoint("pipe-2", PhaseCompleted); err != nil {
		t.Fatalf("basename이 겹쳐도 체크포인트는 성공해야 한다: %v", err)
	}
}

// 재체크포인트가 기존 스냅샷을 파괴하지 않아야 한다(삭제 후 rename 금지).
func TestRecheckpointReplacesSnapshotWithoutLoss(t *testing.T) {
	m, root := newTestManager(t)
	mustCheckpoint(t, m, "pipe-1", PhasePlanned)
	mustWrite(t, filepath.Join(root, ".pylon", "runtime", "pipe-1", "requirement.md"), "# 갱신된 요구사항")
	mustCheckpoint(t, m, "pipe-1", PhasePlanned)

	data, err := os.ReadFile(filepath.Join(root, ".pylon", "history", "pipelines", "pipe-1", "planned", "requirement.md"))
	if err != nil {
		t.Fatalf("교체 후에도 스냅샷이 있어야 한다: %v", err)
	}
	if !strings.Contains(string(data), "갱신된 요구사항") {
		t.Errorf("새 내용으로 교체되어야 한다: %s", data)
	}
	// 교체용 임시 디렉토리가 log에 새어나오면 안 된다.
	entries, err := m.Log("", 20)
	if err != nil || len(entries) != 1 {
		t.Fatalf("체크포인트는 1건이어야 한다: %d, err=%v", len(entries), err)
	}
}

func TestCheckpointCuratesJSONKeys(t *testing.T) {
	m, root := newTestManager(t)
	if _, err := m.Checkpoint("pipe-1", PhaseCompleted); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".pylon", "history", "pipelines", "pipe-1", "completed", "status-summary.json"))
	if err != nil {
		t.Fatalf("status-summary.json이 있어야 한다: %v", err)
	}
	if strings.Contains(string(data), "drop-me") {
		t.Error("allowlist에 없는 키는 제거되어야 한다")
	}
}

// 검증이 수행되지 않아 ok:false가 된 경우, 그 사유(reason)가 이력 요약에 남아야 한다.
// reason이 잘려나가면 요약만 보고는 "실패"와 "검증 미수행"을 구분할 수 없다.
func TestCheckpointKeepsVerificationReason(t *testing.T) {
	m, root := newTestManager(t)
	pipelineDir := filepath.Join(root, ".pylon", "runtime", "pipe-1")
	mustWrite(t, filepath.Join(pipelineDir, "verification.json"),
		`{"ok":false,"checks":[],"skipped":true,"reason":"검증 설정을 찾을 수 없습니다: /w/.pylon/verify.yml","noise":"drop-me"}`)

	if _, err := m.Checkpoint("pipe-1", PhaseCompleted); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(root, ".pylon", "history", "pipelines", "pipe-1", "completed", "verification-summary.json"))
	if err != nil {
		t.Fatalf("verification-summary.json이 있어야 한다: %v", err)
	}
	if !strings.Contains(string(data), "검증 설정을 찾을 수 없습니다") {
		t.Errorf("reason이 요약에 보존되어야 한다: %s", data)
	}
	if strings.Contains(string(data), "drop-me") {
		t.Errorf("allowlist에 없는 키는 제거되어야 한다: %s", data)
	}
}

func TestCheckpointCollectsRepoSubPipelineVerification(t *testing.T) {
	m, root := newTestManager(t)
	pipelineDir := filepath.Join(root, ".pylon", "runtime", "pipe-1")
	mustWrite(t, filepath.Join(pipelineDir, "repos", "service-a", "verification.json"),
		`{"ok":true,"checks":[{"name":"test","ok":true}]}`)
	mustWrite(t, filepath.Join(pipelineDir, "repos", "service-b", "verification.json"),
		`{"ok":false,"checks":[{"name":"test","ok":false}]}`)

	if _, err := m.Checkpoint("pipe-1", PhaseCompleted); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(root, ".pylon", "history", "pipelines", "pipe-1", "completed", "verification-summary.json"))
	if err != nil {
		t.Fatalf("verification-summary.json이 있어야 한다: %v", err)
	}
	var summary struct {
		Records []struct {
			Pipeline string `json:"pipeline"`
		} `json:"records"`
	}
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatal(err)
	}
	if len(summary.Records) != 2 || summary.Records[0].Pipeline != "service-a" || summary.Records[1].Pipeline != "service-b" {
		t.Fatalf("repo verification records = %#v", summary.Records)
	}
}

func TestFailedCheckpointPreservesTrajectoryArtifacts(t *testing.T) {
	m, root := newTestManager(t)
	pipelineDir := filepath.Join(root, ".pylon", "runtime", "pipe-1")
	mustWrite(t, filepath.Join(pipelineDir, "repos", "service-a", "criteria.json"),
		`{"schema_version":1,"run_id":"pipe-1","repo_id":"service-a","base_revision":"abc","acceptance_criteria":[{"id":"AC-1","description":"works"}],"verification":[{"name":"test","kind":"deterministic","command":"go test ./..."}],"digest":"sha256:criteria","secret":"drop-me"}`)
	mustWrite(t, filepath.Join(pipelineDir, "tasks", "T001", "attempts", "003", "task-report.json"),
		`{"schema_version":1,"run_id":"pipe-1","task_id":"T001","attempt":3,"status":"failed","summary":"tests failed","hypotheses_rejected":[{"hypothesis":"wrong path","probe":"resolve path","result":"correct"}],"remaining_unknowns":["race"],"digest":"sha256:report","secret":"drop-me"}`)
	mustWrite(t, filepath.Join(pipelineDir, "failure-record.json"),
		`{"schema_version":1,"run_id":"pipe-1","task_id":"T001","attempt":3,"phase":"verification","terminal_cause":"test_failure","evidence_refs":["tasks/T001/attempts/003/stderr.log"],"hypotheses_rejected":[{"hypothesis":"wrong path","probe":"resolve path","result":"correct"}],"remaining_unknowns":["race"],"abandoned_reason":"attempts exhausted","recorded_at":"2026-08-10T04:00:00Z","digest":"sha256:failure","secret":"drop-me"}`)
	mustWrite(t, filepath.Join(pipelineDir, "repos", "service-a", "evaluator-result.json"),
		`{"schema_version":1,"request_digest":"sha256:req","status":"fail","summary":"criterion partial","criteria":[{"id":"AC-1","status":"partial","evidence":"change.diff"}],"evaluator":"verifier","secret":"drop-me"}`)
	mustWrite(t, filepath.Join(pipelineDir, "tasks", "T001", "state.json"),
		`{"schema_version":1,"run_id":"pipe-1","task_id":"T001","status":"failed","attempt":3,"message":"verification failed","secret":"drop-me"}`)
	mustWrite(t, filepath.Join(pipelineDir, "tasks", "T001", "attempts", "003", "provider.json"),
		`{"provider":"fake","external_id":"worker-3","attempt":3,"secret":"drop-me"}`)

	if _, err := m.Checkpoint("pipe-1", PhaseFailed); err != nil {
		t.Fatal(err)
	}
	snapshotDir := filepath.Join(root, ".pylon", "history", "pipelines", "pipe-1", "failed")
	for _, name := range []string{
		"criteria-summary.json",
		"task-reports-summary.json",
		"failure-records-summary.json",
		"evaluator-summary.json",
		"attempt-state-summary.json",
		"provider-summary.json",
	} {
		data, err := os.ReadFile(filepath.Join(snapshotDir, name))
		if err != nil {
			t.Fatalf("%s missing: %v", name, err)
		}
		if strings.Contains(string(data), "drop-me") {
			t.Fatalf("%s contains uncurated fields: %s", name, data)
		}
	}
	failureSummary, err := os.ReadFile(filepath.Join(snapshotDir, "failure-records-summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"wrong path", "attempts exhausted", "tasks/T001/attempts/003/stderr.log"} {
		if !strings.Contains(string(failureSummary), expected) {
			t.Fatalf("failure trajectory missing %q: %s", expected, failureSummary)
		}
	}
}

func TestLogSortsByRecordedAtDesc(t *testing.T) {
	m, root := newTestManager(t)
	base := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	m.Now = func() time.Time { return base }
	mustCheckpoint(t, m, "pipe-1", PhasePlanned)

	// 두 번째 파이프라인 — 더 늦은 시각, 다른 내용
	mustWrite(t, filepath.Join(root, ".pylon", "runtime", "pipe-2", "requirement.md"), "# 다른 요구사항")
	m.Now = func() time.Time { return base.Add(time.Hour) }
	mustCheckpoint(t, m, "pipe-2", PhasePlanned)

	entries, err := m.Log("", 20)
	if err != nil || len(entries) != 2 {
		t.Fatalf("2건: %v, err=%v", entries, err)
	}
	if entries[0].PipelineID != "pipe-2" {
		t.Errorf("최신이 먼저: %+v", entries)
	}
	filtered, err := m.Log("pipe-1", 20)
	if err != nil || len(filtered) != 1 || filtered[0].PipelineID != "pipe-1" {
		t.Errorf("파이프라인 필터: %+v, err=%v", filtered, err)
	}
}

func TestShowAndExport(t *testing.T) {
	m, _ := newTestManager(t)
	mustCheckpoint(t, m, "pipe-1", PhasePlanned)

	manifest, files, err := m.Show("pipe-1/planned")
	if err != nil || manifest == nil || len(files) == 0 {
		t.Fatalf("Show 실패: %+v, %v, err=%v", manifest, files, err)
	}
	if _, _, err := m.Show("ghost/planned"); err == nil {
		t.Error("없는 ref는 에러여야 한다")
	}

	out := filepath.Join(t.TempDir(), "export")
	if err := m.Export("pipe-1/planned", out); err != nil {
		t.Fatalf("Export 실패: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "requirement.md")); err != nil {
		t.Errorf("export 결과 누락: %v", err)
	}
}

func TestDiffBetweenPhases(t *testing.T) {
	m, root := newTestManager(t)
	mustCheckpoint(t, m, "pipe-1", PhasePlanned)
	mustWrite(t, filepath.Join(root, ".pylon", "runtime", "pipe-1", "requirement.md"), "# 수정된 요구사항")
	mustCheckpoint(t, m, "pipe-1", PhaseExecuted)

	out, err := m.Diff("pipe-1/planned", "pipe-1/executed")
	if err != nil {
		t.Fatalf("Diff 실패: %v", err)
	}
	if !strings.Contains(out, "수정된 요구사항") {
		t.Errorf("diff 출력에 변경 내용이 있어야 한다:\n%s", out)
	}
}
