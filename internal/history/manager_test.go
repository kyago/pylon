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
