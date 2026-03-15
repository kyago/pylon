package git

import (
	"fmt"
	"strings"
	"testing"
)

// MockRunner는 테스트용 CommandRunner 구현체입니다.
type MockRunner struct {
	// RunFunc가 설정되면 Run 호출 시 이 함수를 실행합니다.
	RunFunc func(dir, name string, args ...string) ([]byte, error)
	// Calls는 Run 호출 기록을 저장합니다.
	Calls []MockCall
}

// MockCall은 Run 호출 기록입니다.
type MockCall struct {
	Dir  string
	Name string
	Args []string
}

func (m *MockRunner) Run(dir, name string, args ...string) ([]byte, error) {
	m.Calls = append(m.Calls, MockCall{Dir: dir, Name: name, Args: args})
	if m.RunFunc != nil {
		return m.RunFunc(dir, name, args...)
	}
	return nil, nil
}

func TestCreateBranch_RunnerError(t *testing.T) {
	orig := defaultRunner
	defer func() { defaultRunner = orig }()

	mock := &MockRunner{
		RunFunc: func(dir, name string, args ...string) ([]byte, error) {
			return []byte("error output"), fmt.Errorf("exit status 128")
		},
	}
	defaultRunner = mock

	err := CreateBranch("/tmp/repo", "test-branch")
	if err == nil {
		t.Fatal("CreateBranch should return error when runner fails")
	}
	if !strings.Contains(err.Error(), "failed to create branch") {
		t.Errorf("error should contain 'failed to create branch', got: %v", err)
	}
	if !strings.Contains(err.Error(), "error output") {
		t.Errorf("error should contain runner output, got: %v", err)
	}

	// 호출 인자 검증
	if len(mock.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.Calls))
	}
	call := mock.Calls[0]
	if call.Dir != "/tmp/repo" {
		t.Errorf("dir = %q, want /tmp/repo", call.Dir)
	}
	if call.Name != "git" {
		t.Errorf("name = %q, want git", call.Name)
	}
	expectedArgs := []string{"checkout", "-b", "test-branch"}
	if strings.Join(call.Args, " ") != strings.Join(expectedArgs, " ") {
		t.Errorf("args = %v, want %v", call.Args, expectedArgs)
	}
}

func TestCurrentBranch_RunnerError(t *testing.T) {
	orig := defaultRunner
	defer func() { defaultRunner = orig }()

	mock := &MockRunner{
		RunFunc: func(dir, name string, args ...string) ([]byte, error) {
			return nil, fmt.Errorf("not a git repository")
		},
	}
	defaultRunner = mock

	_, err := CurrentBranch("/tmp/not-a-repo")
	if err == nil {
		t.Fatal("CurrentBranch should return error when runner fails")
	}
	if !strings.Contains(err.Error(), "failed to get current branch") {
		t.Errorf("error should contain 'failed to get current branch', got: %v", err)
	}
}

func TestCurrentBranch_RunnerSuccess(t *testing.T) {
	orig := defaultRunner
	defer func() { defaultRunner = orig }()

	mock := &MockRunner{
		RunFunc: func(dir, name string, args ...string) ([]byte, error) {
			return []byte("main\n"), nil
		},
	}
	defaultRunner = mock

	branch, err := CurrentBranch("/tmp/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if branch != "main" {
		t.Errorf("branch = %q, want 'main'", branch)
	}
}

func TestPushBranch_RunnerError(t *testing.T) {
	orig := defaultRunner
	defer func() { defaultRunner = orig }()

	mock := &MockRunner{
		RunFunc: func(dir, name string, args ...string) ([]byte, error) {
			return []byte("remote error"), fmt.Errorf("exit status 1")
		},
	}
	defaultRunner = mock

	err := PushBranch("/tmp/repo", "feature-branch")
	if err == nil {
		t.Fatal("PushBranch should return error when runner fails")
	}
	if !strings.Contains(err.Error(), "failed to push branch") {
		t.Errorf("error should contain 'failed to push branch', got: %v", err)
	}
}

func TestWorktreeManager_Create_RunnerError(t *testing.T) {
	mock := &MockRunner{
		RunFunc: func(dir, name string, args ...string) ([]byte, error) {
			return []byte("worktree error"), fmt.Errorf("exit status 128")
		},
	}

	wm := &WorktreeManager{
		Enabled: true,
		Runner:  mock,
	}

	// Create a temp dir for the worktree base
	tmpDir := t.TempDir()

	_, err := wm.Create(tmpDir, "test-agent", "task/branch")
	if err == nil {
		t.Fatal("Create should return error when runner fails")
	}
	if !strings.Contains(err.Error(), "failed to create worktree") {
		t.Errorf("error should contain 'failed to create worktree', got: %v", err)
	}
}

func TestExecRunner_Run(t *testing.T) {
	// ExecRunner가 실제 명령을 실행하는지 확인 (echo 사용)
	r := &ExecRunner{}
	output, err := r.Run("", "echo", "hello")
	if err != nil {
		t.Fatalf("ExecRunner.Run failed: %v", err)
	}
	if strings.TrimSpace(string(output)) != "hello" {
		t.Errorf("output = %q, want 'hello'", strings.TrimSpace(string(output)))
	}
}
