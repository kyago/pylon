package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kyago/pylon/internal/layout"
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
			name: "removes AGENTS.md alongside CLAUDE.md",
			input: `node_modules/

# Pylon-generated Claude Code config (dynamically generated)
.claude/
CLAUDE.md
AGENTS.md

dist/
`,
			want: `node_modules/

dist/
`,
		},
		{
			name: "removes the init-written root agent files section",
			input: `node_modules/

# Pylon root agent files (regenerated; AI-authored)
CLAUDE.md
AGENTS.md

dist/
`,
			want: `node_modules/

dist/
`,
		},
		{
			name: "removes the launch-written section (pre-PR workspaces)",
			input: `node_modules/

# Pylon-generated (dynamically generated)
.claude/
CLAUDE.md
AGENTS.md
.pylon/logs/

dist/
`,
			want: `node_modules/

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

// 사용자 내용이 있는 AGENTS.md는 파일째 지우지 않고 pylon 블록만 벗겨낸다.
func TestUninstallStripsPylonBlockKeepingUserContent(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".pylon"), 0755)
	user := "# 우리 팀 규칙\n\n"
	agentsPath := layout.RootAgentsPath(root)
	os.WriteFile(agentsPath, []byte(user+buildAgentsBlock(root, nil)), 0644)

	plan, err := buildUninstallPlan(root, false, false)
	if err != nil {
		t.Fatalf("buildUninstallPlan() error: %v", err)
	}
	if plan.agentsStripPath != agentsPath {
		t.Fatalf("AGENTS.md with user content should be planned for block-strip, got %q", plan.agentsStripPath)
	}
	for _, f := range plan.runtimeFiles {
		if f == agentsPath {
			t.Error("AGENTS.md with user content must not be planned for deletion")
		}
	}

	skipped, err := executeAgentsUninstall(plan.agentsStripPath)
	if err != nil {
		t.Fatalf("executeAgentsUninstall() error: %v", err)
	}
	if skipped {
		t.Fatal("complete managed block was unexpectedly skipped")
	}
	got, _ := os.ReadFile(agentsPath)
	if string(got) != user {
		t.Errorf("user content should survive block strip:\ngot:  %q\nwant: %q", got, user)
	}
}

func TestUninstallKeepsUserAgentsMDWithNonLeadingVersionStamp(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	agentsPath := layout.RootAgentsPath(root)
	original := "# 우리 팀 규칙\n\n스탬프 예시:\n<!-- pylon-usage-version: 1 -->\n"
	if err := os.WriteFile(agentsPath, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	plan, err := buildUninstallPlan(root, false, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range plan.runtimeFiles {
		if path == agentsPath {
			t.Fatal("a non-leading stamp example must not make user AGENTS.md a deletion target")
		}
	}
	if plan.agentsStripPath != "" {
		t.Fatalf("user AGENTS.md without a block must not be stripped: %q", plan.agentsStripPath)
	}
}

func TestUninstallKeepsHandWrittenClaudeMD(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	claudePath := layout.RootClaudePath(root)
	if err := os.WriteFile(claudePath, []byte("# 우리 팀 CLAUDE.md\n\n사용자 규칙\n"), 0644); err != nil {
		t.Fatal(err)
	}

	plan, err := buildUninstallPlan(root, false, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range plan.runtimeFiles {
		if path == claudePath {
			t.Fatal("hand-written CLAUDE.md must not be treated as a pylon runtime artifact")
		}
	}
}

// malformed 블록은 AGENTS.md만 건너뛰고 나머지 uninstall은 계속 진행되어야 한다 —
// 마크다운 파일 하나 때문에 제거 자체가 불가능해지면 안 된다.
func TestBuildUninstallPlanSkipsMalformedAgentsMDAndContinues(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	original := "# 우리 팀 규칙\n\n" + agentsBlockBegin +
		fmt.Sprintf("\n<!-- pylon-usage-version: %d -->\n", pylonUsageVersion) + bootstrapAgentsMDHeading + "\n"
	agentsPath := layout.RootAgentsPath(root)
	if err := os.WriteFile(agentsPath, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	plan, err := buildUninstallPlan(root, false, false)
	if err != nil {
		t.Fatalf("malformed AGENTS.md must not abort uninstall planning: %v", err)
	}
	if plan.workspacePylon == "" {
		t.Error("rest of the uninstall plan must still be built")
	}
	if plan.agentsStripPath != "" {
		t.Errorf("malformed AGENTS.md must not be planned for strip, got %q", plan.agentsStripPath)
	}
	for _, f := range plan.runtimeFiles {
		if f == agentsPath {
			t.Error("malformed AGENTS.md must not be planned for deletion")
		}
	}
	got, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Fatalf("uninstall planning changed user content: %q", got)
	}
}

func TestExecuteUninstallReportsMalformedAgentsMDWasSkipped(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	agentsPath := layout.RootAgentsPath(root)
	if err := os.WriteFile(agentsPath, []byte(agentsBlockBegin+"\n# damaged\n"), 0644); err != nil {
		t.Fatal(err)
	}
	plan, err := buildUninstallPlan(root, false, false)
	if err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := executeUninstall(root, plan); err != nil {
			t.Fatalf("executeUninstall() error: %v", err)
		}
	})
	if strings.Contains(output, "completely removed") {
		t.Fatalf("partial uninstall must not claim complete removal: %q", output)
	}
	if !strings.Contains(output, agentsPath) || !strings.Contains(output, "그대로") {
		t.Fatalf("completion output must identify the skipped AGENTS.md: %q", output)
	}
	if _, err := os.Stat(agentsPath); err != nil {
		t.Fatalf("malformed AGENTS.md must remain: %v", err)
	}
}

func TestExecuteUninstallRechecksAgentsMDBeforeDeleting(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	agentsPath := layout.RootAgentsPath(root)
	if err := os.WriteFile(agentsPath, []byte(buildAgentsBlock(root, nil)), 0644); err != nil {
		t.Fatal(err)
	}
	plan, err := buildUninstallPlan(root, false, false)
	if err != nil {
		t.Fatal(err)
	}

	user := "# 확인 중 추가된 사용자 규칙\n\n"
	if err := os.WriteFile(agentsPath, []byte(user+buildAgentsBlock(root, nil)), 0644); err != nil {
		t.Fatal(err)
	}
	if err := executeUninstall(root, plan); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("AGENTS.md added during confirmation was deleted: %v", err)
	}
	if string(got) != user {
		t.Fatalf("user content added during confirmation was not preserved: %q", got)
	}
}

func TestExecuteUninstallRechecksClaudeMDBeforeDeleting(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	claudePath := layout.RootClaudePath(root)
	if err := os.WriteFile(claudePath, []byte(buildClaudeMDPointer()), 0644); err != nil {
		t.Fatal(err)
	}
	plan, err := buildUninstallPlan(root, false, false)
	if err != nil {
		t.Fatal(err)
	}

	user := "# 확인 중 추가된 CLAUDE 규칙\n"
	if err := os.WriteFile(claudePath, []byte(user), 0644); err != nil {
		t.Fatal(err)
	}
	if err := executeUninstall(root, plan); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("CLAUDE.md changed during confirmation was deleted: %v", err)
	}
	if string(got) != user {
		t.Fatalf("hand-written CLAUDE.md was not preserved: %q", got)
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

	// Create the generated root agent files
	os.WriteFile(layout.RootClaudePath(root), []byte("@AGENTS.md\n"), 0644)
	os.WriteFile(layout.RootAgentsPath(root), []byte(fmt.Sprintf("<!-- pylon-usage-version: %d -->\n# guide", pylonUsageVersion)), 0644)

	// Create .gitignore
	os.WriteFile(filepath.Join(root, ".gitignore"), []byte("# pylon\n.pylon/runtime/\n"), 0644)

	plan, err := buildUninstallPlan(root, false, false)
	if err != nil {
		t.Fatalf("buildUninstallPlan() error: %v", err)
	}

	// Verify runtime files detected
	if len(plan.runtimeFiles) != 3 {
		t.Errorf("expected 3 runtime files (.claude/, CLAUDE.md, AGENTS.md), got %d: %v", len(plan.runtimeFiles), plan.runtimeFiles)
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
}

func TestExecuteUninstall(t *testing.T) {
	// Create a workspace to uninstall
	root := t.TempDir()

	claudeDir := filepath.Join(root, ".claude")
	os.MkdirAll(claudeDir, 0755)
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte("{}"), 0644)

	claudeMD := filepath.Join(root, "CLAUDE.md")
	os.WriteFile(claudeMD, []byte(buildClaudeMDPointer()), 0644)

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

func TestUninstall_RemoveProjects_ClonePath(t *testing.T) {
	requireGit(t)
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".pylon", "config.yml"), []byte("version: \"0.1\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Workspace is NOT a git repo; standalone clone in subdir
	projDir := filepath.Join(ws, "myproj")
	if out, err := exec.Command("git", "init", projDir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	// DiscoverProjects identifies a subdir as a project if it has its own .pylon/
	if err := os.MkdirAll(filepath.Join(projDir, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}

	plan, err := buildUninstallPlan(ws, true, false)
	if err != nil {
		t.Fatalf("buildUninstallPlan: %v", err)
	}
	if len(plan.cloneProjects) != 1 || plan.cloneProjects[0] != "myproj" {
		t.Errorf("expected cloneProjects=[myproj], got %v", plan.cloneProjects)
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
