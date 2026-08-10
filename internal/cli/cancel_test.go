package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// 파일 기반 체크포인트는 외부 도구 없이 성공하므로 cancel은 취소 상태를 기록하고
// cancelled 체크포인트 스냅샷을 남긴다.
func TestRunCancel_V2RecordsCancelledCheckpoint(t *testing.T) {
	root := setupTestWorkspace(t)

	pipelineDir := filepath.Join(root, ".pylon", "runtime", "20260717-test")
	if err := os.MkdirAll(pipelineDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pipelineDir, "requirement.md"), []byte("req"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pipelineDir, "status.json"), []byte(`{"status":"running","branch":"task/test"}`), 0644); err != nil {
		t.Fatal(err)
	}

	prev := flagWorkspace
	flagWorkspace = root
	defer func() { flagWorkspace = prev }()

	if err := runCancel(newCancelCmd(), []string{"20260717-test"}); err != nil {
		t.Fatalf("runCancel failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(pipelineDir, "status.json"))
	if err != nil {
		t.Fatal(err)
	}
	var sj map[string]string
	if err := json.Unmarshal(data, &sj); err != nil {
		t.Fatal(err)
	}
	if sj["status"] != "cancelled" {
		t.Fatalf("status = %q, want cancelled", sj["status"])
	}
	manifest := filepath.Join(root, ".pylon", "history", "pipelines", "20260717-test", "cancelled", "manifest.json")
	if _, err := os.Stat(manifest); err != nil {
		t.Fatalf("cancelled 체크포인트 manifest가 있어야 한다: %v", err)
	}
}

// legacy SQLite 폴백 제거 후, runtime 디렉토리가 없는 알 수 없는 파이프라인은
// 명확한 에러를 반환해야 한다.
func TestRunCancel_UnknownPipelineReturnsError(t *testing.T) {
	root := setupTestWorkspace(t)

	prev := flagWorkspace
	flagWorkspace = root
	defer func() { flagWorkspace = prev }()

	err := runCancel(newCancelCmd(), []string{"no-such-pipeline"})
	if err == nil {
		t.Fatal("expected error for unknown pipeline")
	}
}

// cleanup-pipeline.sh의 runtime 정리 분기를 직접 검증한다:
// 일치하는 terminal checkpoint가 있으면 삭제하고, 없으면 보존한다.
func TestCleanupPipelineScript_RuntimeBranches(t *testing.T) {
	for _, tool := range []string{"bash", "jq", "git"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not available", tool)
		}
	}
	// 스크립트는 저장소 소스 트리에서 직접 가져와 임시 위치에 복사한다
	// (common.sh를 함께 두어 source가 동작하도록).
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	scriptsSrc := filepath.Join(repoRoot, "internal", "cli", "scripts", "bash")
	tmp := t.TempDir()
	for _, name := range []string{"cleanup-pipeline.sh", "common.sh"} {
		data, err := os.ReadFile(filepath.Join(scriptsSrc, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(tmp, name), data, 0755); err != nil {
			t.Fatal(err)
		}
	}
	script := filepath.Join(tmp, "cleanup-pipeline.sh")
	binDir := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	pylonBin := filepath.Join(binDir, "pylon")
	build := exec.Command("go", "build", "-o", pylonBin, "./cmd/pylon")
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build pylon: %v\n%s", err, out)
	}

	// common.sh의 find_repo_root()가 .pylon/ 을 찾을 수 있도록 작업 디렉토리를 준비한다.
	pylonDir := filepath.Join(tmp, ".pylon")
	if err := os.MkdirAll(pylonDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pylonDir, "config.yml"), []byte("version: \"0.1\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	runScript := func(pipelineDir string, terminalPhase ...string) ([]byte, error) {
		t.Helper()
		args := []string{script, pipelineDir}
		if len(terminalPhase) > 0 {
			args = append(args, "--terminal-phase", terminalPhase[0])
		}
		cmd := exec.Command("bash", args...)
		cmd.Dir = tmp
		cmd.Env = append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
		return cmd.CombinedOutput()
	}

	// 분기 1: terminal checkpoint 확인 → 디렉토리 삭제
	deleted := filepath.Join(pylonDir, "runtime", "runtime-deleted")
	if err := os.MkdirAll(deleted, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deleted, "status.json"), []byte(`{"pipeline_id":"runtime-deleted","scope":"root"}`), 0644); err != nil {
		t.Fatal(err)
	}
	checkpoint := exec.Command(pylonBin, "history", "checkpoint", "--pipeline", "runtime-deleted", "--phase", "completed")
	checkpoint.Dir = tmp
	if out, err := checkpoint.CombinedOutput(); err != nil {
		t.Fatalf("create checkpoint: %v\n%s", err, out)
	}
	if out, err := runScript(deleted, "completed"); err != nil {
		t.Fatalf("script failed: %v\n%s", err, out)
	}
	if _, err := os.Stat(deleted); !os.IsNotExist(err) {
		t.Fatalf("runtime dir must be deleted after checkpoint: %v", err)
	}

	// 분기 2: 형식만 맞춘 가짜 manifest는 runtime 삭제를 승인하지 않는다.
	forged := filepath.Join(pylonDir, "runtime", "runtime-forged")
	if err := os.MkdirAll(forged, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(forged, "status.json"), []byte(`{"pipeline_id":"runtime-forged","scope":"root"}`), 0644); err != nil {
		t.Fatal(err)
	}
	forgedManifestDir := filepath.Join(pylonDir, "history", "pipelines", "runtime-forged", "completed")
	if err := os.MkdirAll(forgedManifestDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(forgedManifestDir, "manifest.json"), []byte(`{"pipeline_id":"runtime-forged","phase":"completed"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := runScript(forged, "completed"); err == nil {
		t.Fatalf("forged checkpoint must fail cleanup:\n%s", out)
	}
	if _, err := os.Stat(forged); err != nil {
		t.Fatalf("runtime must survive forged checkpoint: %v", err)
	}

	// 분기 3: checkpoint 미지정 → preserved 마킹 후 보존
	kept := filepath.Join(tmp, "runtime-kept")
	if err := os.MkdirAll(kept, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kept, "status.json"), []byte(`{"pipeline_id":"runtime-kept","scope":"root"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := runScript(kept); err != nil {
		t.Fatalf("script failed: %v\n%s", err, out)
	}
	data, err := os.ReadFile(filepath.Join(kept, "status.json"))
	if err != nil {
		t.Fatalf("status.json must exist when runtime is kept: %v", err)
	}
	var sj struct {
		Cleanup struct {
			Status string `json:"status"`
			Reason string `json:"reason"`
		} `json:"cleanup"`
	}
	if err := json.Unmarshal(data, &sj); err != nil {
		t.Fatal(err)
	}
	if sj.Cleanup.Status != "preserved" || sj.Cleanup.Reason != "terminal_checkpoint_required" {
		t.Fatalf("cleanup = %#v, want preserved checkpoint requirement", sj.Cleanup)
	}

	// 분기 3: 상태가 없는 임의 경로는 정리 대상으로 인정하지 않는다.
	if out, err := runScript(filepath.Join(tmp, "no-such-dir")); err == nil {
		t.Fatalf("script accepted a directory without pipeline state: %s", out)
	}
}
