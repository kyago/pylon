package git

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewWorktreeManager(t *testing.T) {
	tests := []struct {
		name        string
		enabled     bool
		autoCleanup bool
	}{
		{"둘 다 활성화", true, true},
		{"둘 다 비활성화", false, false},
		{"enabled만 활성화", true, false},
		{"autoCleanup만 활성화", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wm := NewWorktreeManager(tt.enabled, tt.autoCleanup)
			if wm == nil {
				t.Fatal("NewWorktreeManager() returned nil")
			}
			if wm.Enabled != tt.enabled {
				t.Errorf("Enabled = %v, want %v", wm.Enabled, tt.enabled)
			}
			if wm.AutoCleanup != tt.autoCleanup {
				t.Errorf("AutoCleanup = %v, want %v", wm.AutoCleanup, tt.autoCleanup)
			}
		})
	}
}

func TestWorktreeManager_Create_Disabled(t *testing.T) {
	wm := NewWorktreeManager(false, false)
	projectDir := "/some/project"

	got, err := wm.Create(projectDir, "agent1", "task/branch")
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if got != projectDir {
		t.Errorf("Create() = %q, want %q (should return projectDir when disabled)", got, projectDir)
	}
}

func TestWorktreeManager_Create_Enabled(t *testing.T) {
	dir := initTestRepo(t)
	wm := NewWorktreeManager(true, false)

	worktreePath, err := wm.Create(dir, "agent1", "task/branch")
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}

	// worktree 경로가 존재하는지 확인
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		t.Errorf("worktree path %q does not exist", worktreePath)
	}

	// 예상 경로 확인
	expectedBase := filepath.Join(dir, ".git", "pylon-worktrees", "agent1")
	if worktreePath != expectedBase {
		t.Errorf("worktree path = %q, want %q", worktreePath, expectedBase)
	}

	// worktree 내부에서 현재 브랜치 확인
	branch, err := CurrentBranch(worktreePath)
	if err != nil {
		t.Fatalf("CurrentBranch() in worktree: %v", err)
	}
	if branch != "task/branch/agent1" {
		t.Errorf("worktree branch = %q, want %q", branch, "task/branch/agent1")
	}
}

func TestWorktreeManager_Remove(t *testing.T) {
	dir := initTestRepo(t)
	wm := NewWorktreeManager(true, false)

	worktreePath, err := wm.Create(dir, "to-remove", "task/rm")
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}

	// worktree 존재 확인
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		t.Fatalf("worktree path should exist before removal")
	}

	// 제거
	if err := wm.Remove(dir, worktreePath); err != nil {
		t.Fatalf("Remove() unexpected error: %v", err)
	}

	// 제거 후 존재하지 않는지 확인
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Errorf("worktree path %q should not exist after removal", worktreePath)
	}
}

func TestWorktreeManager_Cleanup(t *testing.T) {
	dir := initTestRepo(t)
	wm := NewWorktreeManager(true, false)

	// 여러 worktree 생성
	names := []string{"agent-a", "agent-b", "agent-c"}
	for _, name := range names {
		if _, err := wm.Create(dir, name, "task/cleanup"); err != nil {
			t.Fatalf("Create(%q) unexpected error: %v", name, err)
		}
	}

	// worktree 베이스 디렉토리 존재 확인
	worktreeBase := filepath.Join(dir, ".git", "pylon-worktrees")
	entries, err := os.ReadDir(worktreeBase)
	if err != nil {
		t.Fatalf("ReadDir() unexpected error: %v", err)
	}
	if len(entries) != len(names) {
		t.Fatalf("expected %d worktree entries, got %d", len(names), len(entries))
	}

	// Cleanup 실행
	if err := wm.Cleanup(dir); err != nil {
		t.Fatalf("Cleanup() unexpected error: %v", err)
	}

	// 모든 worktree 디렉토리가 제거되었는지 확인
	for _, name := range names {
		path := filepath.Join(worktreeBase, name)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("worktree %q should not exist after cleanup", path)
		}
	}
}

func TestWorktreeManager_Cleanup_NoWorktreeDir(t *testing.T) {
	dir := initTestRepo(t)
	wm := NewWorktreeManager(true, false)

	// pylon-worktrees 디렉토리가 없는 상태에서 Cleanup → 에러 없이 반환
	err := wm.Cleanup(dir)
	if err != nil {
		t.Errorf("Cleanup() unexpected error when no worktree dir: %v", err)
	}
}

func TestSanitizeBranchName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"슬래시 치환", "feature/login", "feature-login"},
		{"공백 치환", "agent name", "agent-name"},
		{"콜론 치환", "agent:1", "agent-1"},
		{"복합 특수문자", "ns/agent:2 test", "ns-agent-2-test"},
		{"특수문자 없음", "simple", "simple"},
		{"빈 문자열", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeBranchName(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeBranchName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
