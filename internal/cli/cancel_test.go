package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// fossil이 없거나 체크포인트가 실패해도 cancel 자체는 성공해야 하고,
// 이때 runtime 디렉토리는 보존된다 (best-effort 원칙).
func TestRunCancel_V2KeepsRuntimeWhenCheckpointFails(t *testing.T) {
	root := setupTestWorkspace(t)
	t.Setenv("PATH", "") // fossil 실행 차단 → 체크포인트 실패 유도

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

	if _, err := os.Stat(pipelineDir); err != nil {
		t.Fatalf("runtime dir must be preserved when checkpoint fails: %v", err)
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
}

// cleanup-pipeline.sh의 runtime 정리 분기를 직접 검증한다:
// 세 번째 인자가 true면 디렉토리 삭제, 아니면 cleaned 마킹 후 보존.
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

	// common.sh의 find_repo_root()가 .pylon/ 을 찾을 수 있도록 작업 디렉토리를 준비한다.
	pylonDir := filepath.Join(tmp, ".pylon")
	if err := os.MkdirAll(pylonDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pylonDir, "config.yml"), []byte("version: \"0.1\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	runScript := func(pipelineDir, deleteRuntime string) {
		t.Helper()
		args := []string{script, pipelineDir, ""}
		if deleteRuntime != "" {
			args = append(args, deleteRuntime)
		}
		cmd := exec.Command("bash", args...)
		cmd.Dir = tmp
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("script failed: %v\n%s", err, out)
		}
	}

	// 분기 1: DELETE_RUNTIME=true → 디렉토리 삭제
	deleted := filepath.Join(tmp, "runtime-deleted")
	if err := os.MkdirAll(deleted, 0755); err != nil {
		t.Fatal(err)
	}
	runScript(deleted, "true")
	if _, err := os.Stat(deleted); !os.IsNotExist(err) {
		t.Fatalf("runtime dir must be deleted when DELETE_RUNTIME=true: %v", err)
	}

	// 분기 2: DELETE_RUNTIME 생략(기본 false) → cleaned 마킹 후 보존
	kept := filepath.Join(tmp, "runtime-kept")
	if err := os.MkdirAll(kept, 0755); err != nil {
		t.Fatal(err)
	}
	runScript(kept, "")
	data, err := os.ReadFile(filepath.Join(kept, "status.json"))
	if err != nil {
		t.Fatalf("status.json must exist when runtime is kept: %v", err)
	}
	var sj map[string]string
	if err := json.Unmarshal(data, &sj); err != nil {
		t.Fatal(err)
	}
	if sj["status"] != "cleaned" {
		t.Fatalf("status = %q, want cleaned", sj["status"])
	}
}
