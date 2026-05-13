package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// requireGit skips the test if the git binary is not available on PATH.
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found on PATH; skipping test")
	}
}

func TestInferProjectName(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://github.com/user/repo.git", "repo"},
		{"https://github.com/user/repo", "repo"},
		{"git@github.com:user/repo.git", "repo"},
		{"git@github.com:user/repo", "repo"},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := inferProjectName(tt.url)
			if got != tt.want {
				t.Errorf("inferProjectName(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestExcludePylonFromRepo(t *testing.T) {
	requireGit(t)
	// Create a temporary git repo to act as the "submodule"
	tmpDir := t.TempDir()
	cmd := exec.Command("git", "init", tmpDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}

	// Run excludePylonFromRepo
	if err := excludePylonFromRepo(tmpDir); err != nil {
		t.Fatalf("excludePylonFromRepo failed: %v", err)
	}

	// Verify .git/info/exclude contains .pylon/
	excludePath := filepath.Join(tmpDir, ".git", "info", "exclude")
	data, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("failed to read exclude file: %v", err)
	}
	if !strings.Contains(string(data), ".pylon/") {
		t.Errorf("exclude file does not contain '.pylon/': %s", string(data))
	}
}

func TestExcludePylonFromRepo_Idempotent(t *testing.T) {
	requireGit(t)
	tmpDir := t.TempDir()
	cmd := exec.Command("git", "init", tmpDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}

	// Call twice
	if err := excludePylonFromRepo(tmpDir); err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	if err := excludePylonFromRepo(tmpDir); err != nil {
		t.Fatalf("second call failed: %v", err)
	}

	// Verify .pylon/ appears only once
	excludePath := filepath.Join(tmpDir, ".git", "info", "exclude")
	data, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("failed to read exclude file: %v", err)
	}
	count := strings.Count(string(data), ".pylon/")
	if count != 1 {
		t.Errorf("expected '.pylon/' to appear once, got %d times in:\n%s", count, string(data))
	}
}

func TestExcludePylonFromRepo_NotGitRepo(t *testing.T) {
	requireGit(t)
	tmpDir := t.TempDir()
	err := excludePylonFromRepo(tmpDir)
	if err == nil {
		t.Fatal("expected error for non-git directory, got nil")
	}
	if !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNewAddProjectCmd_Flags(t *testing.T) {
	cmd := newAddProjectCmd()

	// Verify --force flag exists
	f := cmd.Flags().Lookup("force")
	if f == nil {
		t.Fatal("--force flag not found")
	}
	if f.DefValue != "false" {
		t.Errorf("--force default = %q, want %q", f.DefValue, "false")
	}

	// Verify --skip-clone flag exists
	f = cmd.Flags().Lookup("skip-clone")
	if f == nil {
		t.Fatal("--skip-clone flag not found")
	}
	if f.DefValue != "false" {
		t.Errorf("--skip-clone default = %q, want %q", f.DefValue, "false")
	}

	// Verify --name flag exists
	f = cmd.Flags().Lookup("name")
	if f == nil {
		t.Fatal("--name flag not found")
	}
}

func TestRunAddProject_ForceAndSkipCloneMutuallyExclusive(t *testing.T) {
	// Set up a minimal workspace
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".pylon", "config.yml"), []byte("version: \"0.1\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := newAddProjectCmd()
	cmd.SetArgs([]string{"https://github.com/user/repo.git", "--force", "--skip-clone"})

	// Override workspace flag to our temp dir
	oldWorkspace := flagWorkspace
	flagWorkspace = tmpDir
	defer func() { flagWorkspace = oldWorkspace }()

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for --force + --skip-clone, got nil")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunAddProject_SkipCloneNoDir(t *testing.T) {
	// Set up a minimal workspace
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".pylon", "config.yml"), []byte("version: \"0.1\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := newAddProjectCmd()
	cmd.SetArgs([]string{"https://github.com/user/repo.git", "--skip-clone", "--name", "nonexistent"})

	oldWorkspace := flagWorkspace
	flagWorkspace = tmpDir
	defer func() { flagWorkspace = oldWorkspace }()

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for --skip-clone with missing dir, got nil")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunAddProject_DirExistsNoFlag(t *testing.T) {
	// Set up a minimal workspace with existing project dir
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".pylon", "config.yml"), []byte("version: \"0.1\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "myproject"), 0755); err != nil {
		t.Fatal(err)
	}

	cmd := newAddProjectCmd()
	cmd.SetArgs([]string{"https://github.com/user/myproject.git", "--name", "myproject"})

	oldWorkspace := flagWorkspace
	flagWorkspace = tmpDir
	defer func() { flagWorkspace = oldWorkspace }()

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for existing dir without flags, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("unexpected error: %v", err)
	}
	// Check that the error message includes guidance
	if !strings.Contains(err.Error(), "--force") || !strings.Contains(err.Error(), "--skip-clone") {
		t.Errorf("error should mention --force and --skip-clone: %v", err)
	}
}

func TestRunAddProject_SkipCloneWithExistingDir(t *testing.T) {
	requireGit(t)
	// Set up a workspace with an existing project directory containing a git repo
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".pylon", "config.yml"), []byte("version: \"0.1\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	projectDir := filepath.Join(tmpDir, "myproject")
	cmd := exec.Command("git", "init", projectDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}

	// Create a go.mod so tech stack detection works
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module example.com/myproject\n"), 0644); err != nil {
		t.Fatal(err)
	}

	addCmd := newAddProjectCmd()
	addCmd.SetArgs([]string{"https://github.com/user/myproject.git", "--skip-clone", "--name", "myproject"})

	oldWorkspace := flagWorkspace
	flagWorkspace = tmpDir
	defer func() { flagWorkspace = oldWorkspace }()

	withStdin(t, "n\n", func() {
		if err := addCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// Verify .pylon/ was created inside project
	if _, err := os.Stat(filepath.Join(projectDir, ".pylon", "context.md")); err != nil {
		t.Error("expected .pylon/context.md to be created")
	}
	if _, err := os.Stat(filepath.Join(projectDir, ".pylon", "verify.yml")); err != nil {
		t.Error("expected .pylon/verify.yml to be created")
	}

	// Verify .pylon/ is excluded from git tracking
	excludePath := filepath.Join(projectDir, ".git", "info", "exclude")
	data, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("failed to read exclude file: %v", err)
	}
	if !strings.Contains(string(data), ".pylon/") {
		t.Error("expected .pylon/ to be in git exclude file")
	}
}

func TestValidateProjectName(t *testing.T) {
	valid := []string{"my-project", "repo123", "a"}
	for _, name := range valid {
		if err := validateProjectName(name); err != nil {
			t.Errorf("validateProjectName(%q) unexpected error: %v", name, err)
		}
	}

	invalid := []struct {
		name    string
		wantErr string
	}{
		{"", "cannot be empty"},
		{".", "invalid project name"},
		{"..", "invalid project name"},
		{"a/b", "path separators"},
		{"a\\b", "path separators"},
		{"/abs", "path separators"},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProjectName(tt.name)
			if err == nil {
				t.Fatalf("validateProjectName(%q) expected error, got nil", tt.name)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("validateProjectName(%q) = %v, want error containing %q", tt.name, err, tt.wantErr)
			}
		})
	}
}

func TestRunAddProject_DirExistsAsFile(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".pylon", "config.yml"), []byte("version: \"0.1\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Create a file (not a directory) with the project name
	if err := os.WriteFile(filepath.Join(tmpDir, "myproject"), []byte("not a dir"), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := newAddProjectCmd()
	cmd.SetArgs([]string{"https://github.com/user/myproject.git", "--name", "myproject"})

	oldWorkspace := flagWorkspace
	flagWorkspace = tmpDir
	defer func() { flagWorkspace = oldWorkspace }()

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for file at project path, got nil")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunAddProject_PathTraversal(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".pylon", "config.yml"), []byte("version: \"0.1\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := newAddProjectCmd()
	cmd.SetArgs([]string{"https://github.com/user/repo.git", "--name", "../escape", "--force"})

	oldWorkspace := flagWorkspace
	flagWorkspace = tmpDir
	defer func() { flagWorkspace = oldWorkspace }()

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for path traversal, got nil")
	}
	if !strings.Contains(err.Error(), "path separators") {
		t.Errorf("unexpected error: %v", err)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func withStdin(t *testing.T, input string, fn func()) {
	t.Helper()
	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	if _, err := w.WriteString(input); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	os.Stdin = r
	defer func() {
		os.Stdin = oldStdin
		_ = r.Close()
	}()
	fn()
}

func TestRunAddProject_ForceBlockedOnSubmoduleRemnant(t *testing.T) {
	requireGit(t)
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".pylon", "config.yml"), []byte("version: \"0.1\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// workspace를 git repo로 만들고 .gitmodules에 잔재 추가
	runGit(t, workspace, "init")
	gitmodules := "[submodule \"myproj\"]\n\tpath = myproj\n\turl = https://example.com/myproj.git\n"
	if err := os.WriteFile(filepath.Join(workspace, ".gitmodules"), []byte(gitmodules), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, "myproj"), 0755); err != nil {
		t.Fatal(err)
	}

	cmd := newAddProjectCmd()
	cmd.SetArgs([]string{"https://example.com/myproj.git", "--name", "myproj", "--force"})

	oldWorkspace := flagWorkspace
	flagWorkspace = workspace
	defer func() { flagWorkspace = oldWorkspace }()

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected --force on submodule to be blocked, got nil")
	}
	if !strings.Contains(err.Error(), "migrate-project") {
		t.Errorf("error should suggest migrate-project: %v", err)
	}
}

func TestRunAddProject_UsesPlainClone(t *testing.T) {
	requireGit(t)

	// 워크스페이스(.pylon/만 있음, git 없음)
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".pylon", "config.yml"), []byte("version: \"0.1\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// 더미 origin repo
	origin := t.TempDir()
	if out, err := exec.Command("git", "init", origin).CombinedOutput(); err != nil {
		t.Fatalf("git init origin: %v\n%s", err, out)
	}
	if err := os.WriteFile(filepath.Join(origin, "README.md"), []byte("hi\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, origin, "add", ".")
	runGit(t, origin, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-m", "init")

	addCmd := newAddProjectCmd()
	addCmd.SetArgs([]string{"file://" + origin, "--name", "myproj"})

	oldWorkspace := flagWorkspace
	flagWorkspace = workspace
	defer func() { flagWorkspace = oldWorkspace }()

	withStdin(t, "n\n", func() {
		if err := addCmd.Execute(); err != nil {
			t.Fatalf("add-project failed: %v", err)
		}
	})

	// 1) workspace에 .gitmodules가 생기지 않아야 함
	if _, err := os.Stat(filepath.Join(workspace, ".gitmodules")); err == nil {
		t.Errorf(".gitmodules should not be created in workspace")
	}
	// 2) workspace에 .git/이 자동 생성되지 않아야 함
	if _, err := os.Stat(filepath.Join(workspace, ".git")); err == nil {
		t.Errorf("workspace .git/ should not be created")
	}
	// 3) 하위 디렉토리는 일반 git clone 결과
	subGit := filepath.Join(workspace, "myproj", ".git")
	info, err := os.Stat(subGit)
	if err != nil {
		t.Fatalf("sub project .git missing: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("sub project .git should be a directory (clone), got gitlink file")
	}
	// 4) .pylon/이 sub 디렉토리에 생성됨
	if _, err := os.Stat(filepath.Join(workspace, "myproj", ".pylon", "context.md")); err != nil {
		t.Errorf("expected sub .pylon/context.md, got %v", err)
	}
}

// TestRunAddProject_ForceMigrate_PreservesPylon은 --force --migrate가
// submodule 안의 기존 .pylon/ 내용을 보존하는지 확인한다.
// performMigration 흐름이 .pylon/을 임시 보관·복원하므로 사용자의 context/agents/
// verify 파일이 손실되지 않아야 한다.
func TestRunAddProject_ForceMigrate_PreservesPylon(t *testing.T) {
	requireGit(t)

	ws, name, sub := setupSubmoduleFixture(t)

	// 실제 add-project가 등록했을 .pylon/ exclude를 동일하게 설정 (워킹 트리 dirty 방지)
	if err := excludePylonFromRepo(sub); err != nil {
		t.Fatalf("exclude .pylon/: %v", err)
	}

	// submodule 안에 보존되어야 할 .pylon/ 파일 작성
	if err := os.MkdirAll(filepath.Join(sub, ".pylon", "agents"), 0755); err != nil {
		t.Fatal(err)
	}
	const marker = "preserved-context\n"
	if err := os.WriteFile(filepath.Join(sub, ".pylon", "context.md"), []byte(marker), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, ".pylon", "agents", "developer.md"), []byte("dev-agent"), 0644); err != nil {
		t.Fatal(err)
	}

	// 기존 origin URL (사용자 가정: 동일 URL을 add-project 인자로 넘긴다)
	urlOut, err := exec.Command("git", "-C", sub, "config", "--get", "remote.origin.url").Output()
	if err != nil {
		t.Fatalf("get origin url: %v", err)
	}
	originURL := strings.TrimSpace(string(urlOut))

	cmd := newAddProjectCmd()
	cmd.SetArgs([]string{originURL, "--name", name, "--force", "--migrate"})

	oldWorkspace := flagWorkspace
	flagWorkspace = ws
	defer func() { flagWorkspace = oldWorkspace }()

	if err := cmd.Execute(); err != nil {
		t.Fatalf("add-project --force --migrate failed: %v", err)
	}

	// 1) coupling이 Clone으로 전환됨
	if got := detectProjectCoupling(ws, name); got != CouplingClone {
		t.Errorf("after migration coupling = %v, want CouplingClone", got)
	}
	// 2) 새 디렉토리는 일반 clone 결과 (gitlink 파일이 아닌 .git 디렉토리)
	info, err := os.Stat(filepath.Join(sub, ".git"))
	if err != nil {
		t.Fatalf(".git missing: %v", err)
	}
	if !info.IsDir() {
		t.Errorf(".git should be directory after re-clone, got gitlink")
	}
	// 3) .pylon/context.md가 원래 내용 그대로 보존됨
	data, err := os.ReadFile(filepath.Join(sub, ".pylon", "context.md"))
	if err != nil {
		t.Fatalf("read preserved context: %v", err)
	}
	if string(data) != marker {
		t.Errorf("preserved context lost; got %q, want %q", string(data), marker)
	}
	// 4) agents 디렉토리도 보존됨
	if _, err := os.Stat(filepath.Join(sub, ".pylon", "agents", "developer.md")); err != nil {
		t.Errorf("agent file lost: %v", err)
	}
}
