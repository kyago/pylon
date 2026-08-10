package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubInitDoctorChecks replaces the required-tool gate so init tests run on
// machines (and CI runners) that lack claude/gh.
func stubInitDoctorChecks(t *testing.T, passed bool) {
	t.Helper()
	old := initDoctorChecks
	initDoctorChecks = func() (bool, error) { return passed, nil }
	t.Cleanup(func() { initDoctorChecks = old })
}

// 도구가 없으면 init은 여전히 막혀야 한다 — 게이트 자체의 회귀 방지.
func TestRunInit_BlockedWhenRequiredToolsMissing(t *testing.T) {
	stubInitDoctorChecks(t, false)
	tmp := t.TempDir()
	oldWorkspace := flagWorkspace
	flagWorkspace = tmp
	defer func() { flagWorkspace = oldWorkspace }()

	err := newInitCmd().Execute()
	if err == nil {
		t.Fatal("필수 도구가 없으면 init이 실패해야 한다")
	}
	if !strings.Contains(err.Error(), "required tools are missing") {
		t.Fatalf("예상과 다른 에러: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(tmp, ".pylon")); statErr == nil {
		t.Error("게이트에서 막혔는데 .pylon/이 생성되면 안 된다")
	}
}

func TestRunInit_DoesNotGitInitWorkspace(t *testing.T) {
	requireGit(t)
	stubInitDoctorChecks(t, true)
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
	configData, err := os.ReadFile(filepath.Join(tmp, ".pylon", "config.yml"))
	if err != nil {
		t.Fatal(err)
	}
	configText := string(configData)
	if !strings.Contains(configText, `version: "0.2"`) || !strings.Contains(configText, "provider: auto") {
		t.Fatalf("init did not create provider-neutral config:\n%s", configText)
	}
	if strings.Contains(configText, "backend:") {
		t.Fatalf("new config must not emit deprecated backend:\n%s", configText)
	}
}

func TestInitSetsUpTrackedMemoryDir(t *testing.T) {
	requireGit(t)
	stubInitDoctorChecks(t, true)
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
