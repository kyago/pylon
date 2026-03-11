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

func TestExcludePylonFromSubmodule(t *testing.T) {
	requireGit(t)
	// Create a temporary git repo to act as the "submodule"
	tmpDir := t.TempDir()
	cmd := exec.Command("git", "init", tmpDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}

	// Run excludePylonFromSubmodule
	if err := excludePylonFromSubmodule(tmpDir); err != nil {
		t.Fatalf("excludePylonFromSubmodule failed: %v", err)
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

func TestExcludePylonFromSubmodule_Idempotent(t *testing.T) {
	requireGit(t)
	tmpDir := t.TempDir()
	cmd := exec.Command("git", "init", tmpDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}

	// Call twice
	if err := excludePylonFromSubmodule(tmpDir); err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	if err := excludePylonFromSubmodule(tmpDir); err != nil {
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

func TestExcludePylonFromSubmodule_NotGitRepo(t *testing.T) {
	requireGit(t)
	tmpDir := t.TempDir()
	err := excludePylonFromSubmodule(tmpDir)
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

	// Redirect stdin to auto-answer "n" to agent prompt
	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create stdin pipe: %v", err)
	}
	if _, err := w.WriteString("n\n"); err != nil {
		t.Fatalf("failed to write to stdin pipe: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("failed to close write end of stdin pipe: %v", err)
	}
	os.Stdin = r
	defer func() {
		os.Stdin = oldStdin
		_ = r.Close()
	}()

	oldWorkspace := flagWorkspace
	flagWorkspace = tmpDir
	defer func() { flagWorkspace = oldWorkspace }()

	execErr := addCmd.Execute()
	if execErr != nil {
		t.Fatalf("unexpected error: %v", execErr)
	}

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
