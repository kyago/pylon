package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunInit_DoesNotGitInitWorkspace(t *testing.T) {
	requireGit(t)
	tmp := t.TempDir()
	oldWorkspace := flagWorkspace
	flagWorkspace = tmp
	defer func() { flagWorkspace = oldWorkspace }()

	cmd := newInitCmd()
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, ".git")); err == nil {
		t.Errorf("expected workspace to NOT be a git repo, but .git/ exists")
	}
}

func TestInitSetsUpTrackedMemoryDir(t *testing.T) {
	requireGit(t)
	tmp := t.TempDir()
	oldWorkspace := flagWorkspace
	flagWorkspace = tmp
	defer func() { flagWorkspace = oldWorkspace }()

	cmd := newInitCmd()
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, ".pylon", "memory", ".gitkeep")); err != nil {
		t.Errorf(".pylon/memory/.gitkeep이 생성되어야 한다: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(tmp, ".gitignore"))
	if err != nil {
		t.Fatalf(".gitignore 읽기 실패: %v", err)
	}
	if strings.Contains(string(data), ".pylon/memory") {
		t.Error(".pylon/memory는 git 추적 대상이어야 한다 — gitignore에 없어야 함 (D1)")
	}
	if !strings.Contains(string(data), ".pylon/history/") {
		t.Error(".pylon/history/는 계속 무시되어야 한다")
	}
}
