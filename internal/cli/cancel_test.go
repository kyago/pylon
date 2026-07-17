package cli

import (
	"encoding/json"
	"os"
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
