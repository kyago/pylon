package cli

import (
	"os"
	"path/filepath"
	"slices"
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
	if !strings.Contains(err.Error(), "필수 도구가 없습니다") {
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
	if !strings.Contains(configText, `version: "0.2"`) {
		t.Fatalf("init did not create config:\n%s", configText)
	}
	if strings.Contains(configText, "backend:") {
		t.Fatalf("new config must not emit deprecated backend:\n%s", configText)
	}
}

// 루트가 git 저장소가 아니면 .gitignore를 읽을 주체가 없다. git 전용 파일을 만들지 않는다.
func TestInit_NonGitRootGetsNoGitFiles(t *testing.T) {
	requireGit(t)
	stubInitDoctorChecks(t, true)
	tmp := t.TempDir()
	oldWorkspace := flagWorkspace
	flagWorkspace = tmp
	defer func() { flagWorkspace = oldWorkspace }()

	if err := newInitCmd().Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	for _, name := range []string{".gitignore", filepath.Join(".pylon", "memory", ".gitkeep")} {
		if _, err := os.Stat(filepath.Join(tmp, name)); !os.IsNotExist(err) {
			t.Errorf("git 저장소가 아닌 루트에 %s가 생기면 안 된다 (stat err: %v)", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(tmp, ".pylon", "memory")); err != nil {
		t.Errorf(".pylon/memory/가 생성되어야 한다: %v", err)
	}
}

// git 저장소인 루트에서는 생성물만 ignore하고 메모리는 추적 대상으로 남긴다.
func TestInit_GitRootIgnoresGeneratedFilesOnly(t *testing.T) {
	requireGit(t)
	stubInitDoctorChecks(t, true)
	tmp := t.TempDir()
	runGit(t, tmp, "init", "-q")
	oldWorkspace := flagWorkspace
	flagWorkspace = tmp
	defer func() { flagWorkspace = oldWorkspace }()

	if err := newInitCmd().Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(tmp, ".gitignore"))
	if err != nil {
		t.Fatalf(".gitignore 읽기 실패: %v", err)
	}
	lines := strings.Split(string(data), "\n")
	for _, want := range []string{".claude/", "CLAUDE.md", "AGENTS.md", ".pylon/builtin/"} {
		if !slices.Contains(lines, want) {
			t.Errorf(".gitignore에 %s 줄이 있어야 한다:\n%s", want, data)
		}
	}
	if strings.Contains(string(data), ".pylon/memory") {
		t.Errorf(".pylon/memory는 ignore되면 안 된다:\n%s", data)
	}
}
