package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanGitignoreFull(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "removes pylon runtime section",
			input: `# Other stuff
node_modules/

# Pylon runtime (agent communication, state)
.pylon/runtime/
.pylon/conversations/

# More stuff
dist/
`,
			want: `# Other stuff
node_modules/

# More stuff
dist/
`,
		},
		{
			name: "removes pylon-generated claude section",
			input: `node_modules/

# Pylon-generated Claude Code config (dynamically generated)
.claude/
CLAUDE.md

dist/
`,
			want: `node_modules/

dist/
`,
		},
		{
			name: "removes both pylon sections",
			input: `# deps
node_modules/

# Pylon runtime (agent communication, state)
.pylon/runtime/
.pylon/conversations/

# Pylon-generated Claude Code config (dynamically generated)
.claude/
CLAUDE.md

# build
dist/
`,
			want: `# deps
node_modules/

# build
dist/
`,
		},
		{
			name:  "no pylon entries",
			input: "node_modules/\ndist/\n",
			want:  "node_modules/\ndist/\n",
		},
		{
			name: "removes legacy pylon section marker",
			input: `# pylon
.pylon/runtime/
.pylon/conversations/

other/
`,
			want: `other/
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			path := filepath.Join(tmpDir, ".gitignore")
			if err := os.WriteFile(path, []byte(tt.input), 0644); err != nil {
				t.Fatal(err)
			}

			if err := cleanGitignoreFull(path); err != nil {
				t.Fatalf("cleanGitignoreFull() error: %v", err)
			}

			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			if string(got) != tt.want {
				t.Errorf("cleanGitignoreFull():\ngot:\n%s\nwant:\n%s", string(got), tt.want)
			}
		})
	}
}

func TestCleanGitignoreFull_NonExistent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent", ".gitignore")
	if err := cleanGitignoreFull(path); err != nil {
		t.Errorf("cleanGitignoreFull() on non-existent file should not error, got: %v", err)
	}
}

func TestBuildUninstallPlan(t *testing.T) {
	// Create a minimal workspace structure
	root := t.TempDir()

	// Create .pylon/ directory
	pylonDir := filepath.Join(root, ".pylon")
	os.MkdirAll(filepath.Join(pylonDir, "runtime"), 0755)
	os.WriteFile(filepath.Join(pylonDir, "config.yml"), []byte("workspace:\n  name: test\n"), 0644)

	// Create .claude/ directory
	claudeDir := filepath.Join(root, ".claude")
	os.MkdirAll(claudeDir, 0755)

	// Create CLAUDE.md
	os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("# Test"), 0644)

	// Create .gitignore
	os.WriteFile(filepath.Join(root, ".gitignore"), []byte("# pylon\n.pylon/runtime/\n"), 0644)

	plan, err := buildUninstallPlan(root, false, false)
	if err != nil {
		t.Fatalf("buildUninstallPlan() error: %v", err)
	}

	// Verify runtime files detected
	if len(plan.runtimeFiles) != 2 {
		t.Errorf("expected 2 runtime files (.claude/ and CLAUDE.md), got %d", len(plan.runtimeFiles))
	}

	// Verify workspace pylon detected
	if plan.workspacePylon == "" {
		t.Error("expected workspacePylon to be set")
	}

	// Verify gitignore detected
	if plan.gitignorePath == "" {
		t.Error("expected gitignorePath to be set")
	}

	// Verify no binary (not requested)
	if plan.binaryPath != "" {
		t.Error("expected binaryPath to be empty when removeBinary=false")
	}

	// Verify no submodules (not requested)
	if len(plan.submodules) != 0 {
		t.Errorf("expected no submodules, got %d", len(plan.submodules))
	}
}

func TestExecuteUninstall(t *testing.T) {
	// Create a workspace to uninstall
	root := t.TempDir()

	claudeDir := filepath.Join(root, ".claude")
	os.MkdirAll(claudeDir, 0755)
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte("{}"), 0644)

	claudeMD := filepath.Join(root, "CLAUDE.md")
	os.WriteFile(claudeMD, []byte("# Test"), 0644)

	pylonDir := filepath.Join(root, ".pylon")
	os.MkdirAll(filepath.Join(pylonDir, "runtime", "inbox"), 0755)
	os.MkdirAll(filepath.Join(pylonDir, "agents"), 0755)
	os.WriteFile(filepath.Join(pylonDir, "config.yml"), []byte("workspace:\n  name: test\n"), 0644)

	gitignorePath := filepath.Join(root, ".gitignore")
	os.WriteFile(gitignorePath, []byte("# Pylon runtime (agent communication, state)\n.pylon/runtime/\n.pylon/conversations/\n\nnode_modules/\n"), 0644)

	plan := &uninstallPlan{
		runtimeFiles:   []string{claudeDir, claudeMD},
		workspacePylon: pylonDir,
		gitignorePath:  gitignorePath,
	}

	if err := executeUninstall(root, plan); err != nil {
		t.Fatalf("executeUninstall() error: %v", err)
	}

	// Verify .claude/ removed
	if dirExists(claudeDir) {
		t.Error(".claude/ should be removed")
	}

	// Verify CLAUDE.md removed
	if fileExists(claudeMD) {
		t.Error("CLAUDE.md should be removed")
	}

	// Verify .pylon/ removed
	if dirExists(pylonDir) {
		t.Error(".pylon/ should be removed")
	}

	// Verify .gitignore cleaned
	data, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}
	if strings.Contains(string(data), ".pylon/") {
		t.Errorf(".gitignore should not contain pylon entries, got: %s", string(data))
	}
	if !strings.Contains(string(data), "node_modules/") {
		t.Error(".gitignore should preserve non-pylon entries")
	}
}

func TestFindPylonBinary(t *testing.T) {
	// This test just verifies the function doesn't panic
	// Actual binary location depends on the environment
	path, err := findPylonBinary()
	if err != nil {
		// It's OK if pylon is not installed in test environment
		t.Logf("findPylonBinary() returned error (expected in test env): %v", err)
		return
	}
	if path == "" {
		t.Error("findPylonBinary() returned empty path with no error")
	}
}
