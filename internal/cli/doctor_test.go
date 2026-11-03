package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/layout"
)

func TestSyncBuiltinResources_WritesEmbedsAndIsIdempotent(t *testing.T) {
	root := t.TempDir()
	wrote, err := syncBuiltinResources(root)
	if err != nil {
		t.Fatal(err)
	}
	if !wrote {
		t.Fatal("first sync: wrote=false, want true")
	}
	files, err := embeddedBuiltinFiles()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		path := filepath.Join(layout.BuiltinDir(root), filepath.FromSlash(f.rel))
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, f.content) {
			t.Fatalf("%s not written from embed (err=%v)", f.rel, err)
		}
		if strings.HasSuffix(f.rel, ".sh") {
			if info, _ := os.Stat(path); info.Mode().Perm()&0o100 == 0 {
				t.Errorf("%s is not executable", f.rel)
			}
		}
	}
	if _, err := os.Stat(filepath.Join(layout.BuiltinResourceDir(root, "reference"), "pylon-usage.md")); err != nil {
		t.Errorf("usage manual missing: %v", err)
	}

	wrote, err = syncBuiltinResources(root)
	if err != nil || wrote {
		t.Errorf("second sync: wrote=%v err=%v, want false/nil", wrote, err)
	}
}

// 사용자 디렉터리의 파일은 내장과 이름이 같아도 건드리지 않는다. 읽는 쪽이 내장을 먼저 고른다.
func TestSyncBuiltinResources_LeavesUserFilesAlone(t *testing.T) {
	root := t.TempDir()
	sameName := filepath.Join(layout.ResourceDir(root, "agents"), "dev.md")
	mine := filepath.Join(layout.ResourceDir(root, "agents"), "my-agent.md")
	if err := os.MkdirAll(filepath.Dir(mine), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{sameName, mine} {
		if err := os.WriteFile(path, []byte("mine"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := syncBuiltinResources(root); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{sameName, mine} {
		if got, err := os.ReadFile(path); err != nil || string(got) != "mine" {
			t.Errorf("%s touched: %q, %v", path, got, err)
		}
	}
}

// pylon doctor/update도 .claude/settings.json 훅을 현재 hooks.json으로 갱신한다 — 구버전 Stop 훅이
// `claude`를 직접 띄운 세션에서 매 턴 실패하지 않도록. 사용자 훅은 남는다.
func TestSyncGeneratedFiles_RefreshesSettingsHooks(t *testing.T) {
	root := setupTestWorkspace(t)
	claudeDir := layout.ClaudeDir(root)
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := `{"hooks":{
  "SessionStart":[{"matcher":"stale","hooks":[{"type":"command","command":"pylon mem inject"}]}],
  "PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"my-guard"}]}]
}}`
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}
	oldWorkspace := flagWorkspace
	flagWorkspace = root
	t.Cleanup(func() { flagWorkspace = oldWorkspace })

	captureStdout(t, func() { syncGeneratedFilesIfWorkspace(bytes.NewBuffer(nil), true) })

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	var settings struct {
		Hooks map[string][]settingsHookEntry `json:"hooks"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	if start := settings.Hooks["SessionStart"]; len(start) != 1 || start[0].Matcher == "stale" {
		t.Errorf("SessionStart hook not refreshed: %s", data)
	}
	if pre := settings.Hooks["PreToolUse"]; len(pre) != 1 || pre[0].Hooks[0].Command != "my-guard" {
		t.Errorf("user hook must be kept: %s", data)
	}
}

// builtin은 매번 파일 단위로 맞춘다: 지워지거나 고쳐진 파일은 복구하고, 내장에 없는 파일은
// 지운다(바이너리에서 빠진 리소스). 이미 맞으면 아무것도 바꾸지 않는다.
func TestSyncBuiltinResources_RepairsAndPrunesBuiltin(t *testing.T) {
	root := t.TempDir()
	if _, err := syncBuiltinResources(root); err != nil {
		t.Fatal(err)
	}
	common := filepath.Join(layout.BuiltinResourceDir(root, "commands"), "pl-cleanup.md")
	builtinAgent := filepath.Join(layout.BuiltinResourceDir(root, "agents"), "dev.md")
	dropped := filepath.Join(layout.BuiltinResourceDir(root, "agents"), "dropped-in-new-release.md")
	if err := os.Remove(common); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(builtinAgent, []byte("edited by a session"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dropped, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := syncBuiltinResources(root)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Error("a damaged builtin must be reported as changed")
	}
	if _, err := os.Stat(common); err != nil {
		t.Errorf("deleted file not restored: %v", err)
	}
	want, _ := embeddedAgents.ReadFile("agents/dev.md")
	if got, _ := os.ReadFile(builtinAgent); !bytes.Equal(got, want) {
		t.Error("edited builtin file not restored")
	}
	if _, err := os.Stat(dropped); !os.IsNotExist(err) {
		t.Error("resource absent from the embeds survived")
	}
	if changed, err := syncBuiltinResources(root); err != nil || changed {
		t.Errorf("repaired builtin must be stable: changed=%v err=%v", changed, err)
	}
}

func TestCheckExcludeStatus(t *testing.T) {
	requireGit(t)
	tmpDir := t.TempDir()
	cmd := exec.Command("git", "init", tmpDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}

	// Before adding entry: git repo, no entry
	isGit, hasEntry := checkExcludeStatus(tmpDir)
	if !isGit {
		t.Error("expected isGitRepo=true")
	}
	if hasEntry {
		t.Error("expected hasEntry=false before adding entry")
	}

	// Add .pylon/ to exclude
	if err := excludePylonFromRepo(tmpDir); err != nil {
		t.Fatalf("excludePylonFromRepo failed: %v", err)
	}

	// After adding entry: git repo, has entry
	isGit, hasEntry = checkExcludeStatus(tmpDir)
	if !isGit {
		t.Error("expected isGitRepo=true")
	}
	if !hasEntry {
		t.Error("expected hasEntry=true after adding entry")
	}
}

func TestCheckExcludeStatus_NotGitRepo(t *testing.T) {
	tmpDir := t.TempDir()
	isGit, hasEntry := checkExcludeStatus(tmpDir)
	if isGit {
		t.Error("expected isGitRepo=false for non-git directory")
	}
	if hasEntry {
		t.Error("expected hasEntry=false for non-git directory")
	}
}

func TestCheckExcludeStatus_NoExcludeFile(t *testing.T) {
	requireGit(t)
	tmpDir := t.TempDir()
	cmd := exec.Command("git", "init", tmpDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}

	// Remove the exclude file if it exists
	excludePath := filepath.Join(tmpDir, ".git", "info", "exclude")
	os.Remove(excludePath)

	isGit, hasEntry := checkExcludeStatus(tmpDir)
	if !isGit {
		t.Error("expected isGitRepo=true")
	}
	if hasEntry {
		t.Error("expected hasEntry=false when exclude file is missing")
	}
}

func TestCheckExcludeStatus_WithOtherEntries(t *testing.T) {
	requireGit(t)
	tmpDir := t.TempDir()
	cmd := exec.Command("git", "init", tmpDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}

	// Write exclude file with other entries but NOT .pylon/
	excludePath := filepath.Join(tmpDir, ".git", "info", "exclude")
	content := strings.Join([]string{
		"# git ls-files --others --exclude-from=.git/info/exclude",
		"*.log",
		"node_modules/",
		"",
	}, "\n")
	if err := os.WriteFile(excludePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	isGit, hasEntry := checkExcludeStatus(tmpDir)
	if !isGit {
		t.Error("expected isGitRepo=true")
	}
	if hasEntry {
		t.Error("expected hasEntry=false when .pylon/ is not in exclude")
	}

	// Now add .pylon/ and verify
	if err := excludePylonFromRepo(tmpDir); err != nil {
		t.Fatal(err)
	}
	isGit, hasEntry = checkExcludeStatus(tmpDir)
	if !isGit || !hasEntry {
		t.Error("expected isGitRepo=true, hasEntry=true after adding .pylon/")
	}
}

func TestCheckRepoExcludes_Fix(t *testing.T) {
	requireGit(t)

	// Set up a workspace with a project that has .pylon/ but no exclude entry
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
	if err := os.MkdirAll(filepath.Join(projectDir, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}

	// Verify project is discoverable (explicit precondition)
	projects, err := config.DiscoverProjects(tmpDir)
	if err != nil {
		t.Fatalf("DiscoverProjects failed: %v", err)
	}
	if len(projects) == 0 {
		t.Fatal("precondition failed: no projects discovered")
	}

	// Remove exclude file to ensure .pylon/ is NOT excluded
	excludePath := filepath.Join(projectDir, ".git", "info", "exclude")
	os.Remove(excludePath)

	// Verify .pylon/ is not excluded
	_, hasEntry := checkExcludeStatus(projectDir)
	if hasEntry {
		t.Fatal("precondition failed: .pylon/ should not be in exclude yet")
	}

	// Run checkRepoExcludes with fix=true
	oldWorkspace := flagWorkspace
	flagWorkspace = tmpDir
	defer func() { flagWorkspace = oldWorkspace }()

	ok := checkRepoExcludes(true)
	if !ok {
		t.Error("expected checkRepoExcludes to return true after successful fix")
	}

	// Verify .pylon/ is now excluded
	_, hasEntry = checkExcludeStatus(projectDir)
	if !hasEntry {
		t.Error("expected .pylon/ to be in exclude after fix")
	}
}

func TestCheckRepoExcludes_DetectMissing(t *testing.T) {
	requireGit(t)

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
	if err := os.MkdirAll(filepath.Join(projectDir, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}

	excludePath := filepath.Join(projectDir, ".git", "info", "exclude")
	os.Remove(excludePath)

	oldWorkspace := flagWorkspace
	flagWorkspace = tmpDir
	defer func() { flagWorkspace = oldWorkspace }()

	// fix=false should report missing and return false
	ok := checkRepoExcludes(false)
	if ok {
		t.Error("expected checkRepoExcludes to return false when excludes are missing")
	}

	// Should still NOT be excluded (fix=false)
	_, hasEntry := checkExcludeStatus(projectDir)
	if hasEntry {
		t.Error("expected .pylon/ to remain missing when fix=false")
	}
}

func TestCheckRepoExcludes_AllOK(t *testing.T) {
	requireGit(t)

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
	if err := os.MkdirAll(filepath.Join(projectDir, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}

	// Pre-seed the exclude entry
	if err := excludePylonFromRepo(projectDir); err != nil {
		t.Fatal(err)
	}

	oldWorkspace := flagWorkspace
	flagWorkspace = tmpDir
	defer func() { flagWorkspace = oldWorkspace }()

	ok := checkRepoExcludes(false)
	if !ok {
		t.Error("expected checkRepoExcludes to return true when all projects have excludes")
	}

	// Verify entry still intact
	_, hasEntry := checkExcludeStatus(projectDir)
	if !hasEntry {
		t.Error("expected .pylon/ to remain in exclude")
	}
}

func TestCheckRepoExcludes_SkipsNonGitProjects(t *testing.T) {
	requireGit(t)

	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".pylon", "config.yml"), []byte("version: \"0.1\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a non-git project with .pylon/
	projectDir := filepath.Join(tmpDir, "non-git-project")
	if err := os.MkdirAll(filepath.Join(projectDir, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}

	oldWorkspace := flagWorkspace
	flagWorkspace = tmpDir
	defer func() { flagWorkspace = oldWorkspace }()

	// Should return true (non-git projects are skipped, not flagged as missing)
	ok := checkRepoExcludes(false)
	if !ok {
		t.Error("expected checkRepoExcludes to return true when only non-git projects exist")
	}
}

// newTestWorkspace creates a minimal pylon workspace (empty .pylon/commands/)
// and points flagWorkspace at it for the duration of the test.
func newTestWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".pylon", "commands"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".pylon", "config.yml"), []byte("version: \"0.1\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	old := flagWorkspace
	flagWorkspace = root
	t.Cleanup(func() { flagWorkspace = old })
	return root
}

// TestBuildDesiredClaudeCommands_Keyset verifies the desired command set equals
// exactly the dynamic slash commands plus the embedded pl-* commands (prefix
// stripped). This guards the shared source of truth used by launch and doctor.
func TestBuildDesiredClaudeCommands_Keyset(t *testing.T) {
	root := newTestWorkspace(t) // built-in pl-* come straight from the embeds

	desired := buildDesiredClaudeCommands(root)

	want := map[string]bool{}
	embedded, err := embeddedCommands.ReadDir("commands")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range embedded {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		want[filepath.Join("pl", strings.TrimPrefix(e.Name(), "pl-"))] = true
	}

	if len(desired) != len(want) {
		t.Errorf("desired has %d files, want %d", len(desired), len(want))
	}
	for k := range want {
		if _, ok := desired[k]; !ok {
			t.Errorf("desired missing expected key %q", k)
		}
	}
	for k := range desired {
		if !want[k] {
			t.Errorf("desired has unexpected key %q", k)
		}
	}
}

// 사용자 커맨드(.pylon/commands/)는 추가되지만, 내장 이름과 겹치면 내장 내용이 이긴다.
func TestBuildDesiredClaudeCommands_UserCommands(t *testing.T) {
	root := newTestWorkspace(t)
	userDir := layout.ResourceDir(root, "commands")
	for name, body := range map[string]string{"pl-mine.md": "# mine\n", "pl-cleanup.md": "# shadow\n"} {
		if err := os.WriteFile(filepath.Join(userDir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	desired := buildDesiredClaudeCommands(root)
	if desired[filepath.Join("pl", "mine.md")] != "# mine\n" {
		t.Error("user command missing from desired set")
	}
	want, _ := embeddedCommands.ReadFile("commands/pl-cleanup.md")
	if desired[filepath.Join("pl", "cleanup.md")] != string(want) {
		t.Error("user file must not shadow a built-in command")
	}
}

func TestDiffClaudeCommands(t *testing.T) {
	root := newTestWorkspace(t)
	commandsDir := filepath.Join(root, ".claude", "commands")
	desired := buildDesiredClaudeCommands(root)

	// Nothing on disk -> everything is an addition, nothing changed/removed.
	diff := diffClaudeCommands(commandsDir, desired)
	if len(diff.added) != len(desired) {
		t.Errorf("added = %d, want %d", len(diff.added), len(desired))
	}
	if len(diff.changed) != 0 || len(diff.removed) != 0 {
		t.Errorf("expected no changed/removed, got changed=%d removed=%d", len(diff.changed), len(diff.removed))
	}

	// Apply, then a stale edit + a user file -> one changed, the user file is not flagged.
	if err := applyClaudeCommands(commandsDir, desired); err != nil {
		t.Fatal(err)
	}
	staleTarget := filepath.Join("pl", "cleanup.md")
	if err := os.WriteFile(filepath.Join(commandsDir, staleTarget), []byte("STALE\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(commandsDir, "status.md"), []byte("mine\n"), 0644); err != nil {
		t.Fatal(err)
	}

	diff = diffClaudeCommands(commandsDir, desired)
	if len(diff.added) != 0 {
		t.Errorf("expected no additions, got %v", diff.added)
	}
	if len(diff.changed) != 1 || diff.changed[0] != staleTarget {
		t.Errorf("changed = %v, want [%s]", diff.changed, staleTarget)
	}
	if len(diff.removed) != 0 {
		t.Errorf("removed = %v, want none", diff.removed)
	}
}

func TestDiffClaudeCommands_RemovesPreviouslyManagedCommand(t *testing.T) {
	root := newTestWorkspace(t)
	commandsDir := filepath.Join(root, ".claude", "commands")
	initial := map[string]string{
		filepath.Join("pl", "old.md"):  "old\n",
		filepath.Join("pl", "keep.md"): "keep\n",
	}
	if err := applyClaudeCommands(commandsDir, initial); err != nil {
		t.Fatal(err)
	}
	custom := filepath.Join(commandsDir, "pl", "custom.md")
	if err := os.WriteFile(custom, []byte("custom\n"), 0644); err != nil {
		t.Fatal(err)
	}

	desired := map[string]string{
		filepath.Join("pl", "keep.md"): "keep\n",
	}
	diff := diffClaudeCommands(commandsDir, desired)
	if len(diff.removed) != 1 || diff.removed[0] != filepath.Join("pl", "old.md") {
		t.Fatalf("removed = %v, want [pl/old.md]", diff.removed)
	}
	if err := applyClaudeCommands(commandsDir, desired); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(commandsDir, "pl", "old.md")); !os.IsNotExist(err) {
		t.Errorf("expected previously managed command to be removed, got err=%v", err)
	}
	if _, err := os.Stat(custom); err != nil {
		t.Errorf("expected custom command to be preserved: %v", err)
	}
}

func TestSyncClaudeCommands_ConsentApplies(t *testing.T) {
	root := newTestWorkspace(t)
	commandsDir := filepath.Join(root, ".claude", "commands")

	syncClaudeCommandsIfWorkspace(bytes.NewBufferString("y\n"), false)

	if _, err := os.Stat(filepath.Join(commandsDir, "pl", "cleanup.md")); err != nil {
		t.Errorf("expected pl/cleanup.md to be created after consent: %v", err)
	}
}

func TestSyncClaudeCommands_DeclineDoesNothing(t *testing.T) {
	root := newTestWorkspace(t)
	commandsDir := filepath.Join(root, ".claude", "commands")

	syncClaudeCommandsIfWorkspace(bytes.NewBufferString("n\n"), false)

	if _, err := os.Stat(filepath.Join(commandsDir, "pl", "cleanup.md")); !os.IsNotExist(err) {
		t.Errorf("expected no commands written after decline, got err=%v", err)
	}
}

// TestSyncClaudeCommands_EmptyInputDeclines verifies default = No on EOF.
func TestSyncClaudeCommands_EmptyInputDeclines(t *testing.T) {
	root := newTestWorkspace(t)
	commandsDir := filepath.Join(root, ".claude", "commands")

	syncClaudeCommandsIfWorkspace(bytes.NewBufferString(""), false)

	if _, err := os.Stat(filepath.Join(commandsDir, "pl", "cleanup.md")); !os.IsNotExist(err) {
		t.Errorf("expected no commands written on empty input, got err=%v", err)
	}
}

// TestSyncClaudeCommands_AutoYesSkipsPrompt applies without reading stdin.
func TestSyncClaudeCommands_AutoYesSkipsPrompt(t *testing.T) {
	root := newTestWorkspace(t)
	commandsDir := filepath.Join(root, ".claude", "commands")

	syncClaudeCommandsIfWorkspace(bytes.NewBufferString(""), true)

	if _, err := os.Stat(filepath.Join(commandsDir, "pl", "cleanup.md")); err != nil {
		t.Errorf("expected commands written with autoYes, got err=%v", err)
	}
}

func TestNewDoctorCmd_YesFlag(t *testing.T) {
	cmd := newDoctorCmd()
	f := cmd.Flags().Lookup("yes")
	if f == nil {
		t.Fatal("--yes flag not found")
	}
	if f.DefValue != "false" {
		t.Errorf("--yes default = %q, want %q", f.DefValue, "false")
	}
}

func TestNewDoctorCmd_FixExcludesFlag(t *testing.T) {
	cmd := newDoctorCmd()

	f := cmd.Flags().Lookup("fix-excludes")
	if f == nil {
		t.Fatal("--fix-excludes flag not found")
	}
	if f.DefValue != "false" {
		t.Errorf("--fix-excludes default = %q, want %q", f.DefValue, "false")
	}
}

func TestResolveGitExcludePath(t *testing.T) {
	requireGit(t)
	tmpDir := t.TempDir()
	cmd := exec.Command("git", "init", tmpDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}

	path, err := resolveGitExcludePath(tmpDir)
	if err != nil {
		t.Fatalf("resolveGitExcludePath failed: %v", err)
	}
	if !strings.HasSuffix(path, filepath.Join("info", "exclude")) {
		t.Errorf("expected path ending with info/exclude, got %s", path)
	}
}

func TestResolveGitExcludePath_NotGitRepo(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := resolveGitExcludePath(tmpDir)
	if err == nil {
		t.Fatal("expected error for non-git directory, got nil")
	}
	if !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInstallHint_BrewFormulaWithBrew(t *testing.T) {
	c := Check{Name: "sampletool", InstallURL: "https://example.com/install", BrewFormula: "sampletool"}
	got := installHint(c, true)
	want := "brew install sampletool (또는 https://example.com/install)"
	if got != want {
		t.Errorf("installHint = %q, want %q", got, want)
	}
}

func TestInstallHint_BrewFormulaWithoutBrew(t *testing.T) {
	c := Check{Name: "sampletool", InstallURL: "https://example.com/install", BrewFormula: "sampletool"}
	if got := installHint(c, false); got != c.InstallURL {
		t.Errorf("installHint = %q, want %q", got, c.InstallURL)
	}
}

func TestInstallHint_NoBrewFormula(t *testing.T) {
	c := Check{Name: "git", InstallURL: "https://git-scm.com/downloads"}
	if got := installHint(c, true); got != c.InstallURL {
		t.Errorf("installHint = %q, want %q", got, c.InstallURL)
	}
}

func TestReconcileRootAgentFilesLeavesCurrent(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(layout.PylonDir(root), 0755); err != nil {
		t.Fatal(err)
	}
	current := buildAgentsBlock(nil) + "\n# 사용자 규칙\n"
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte(current), 0644); err != nil {
		t.Fatal(err)
	}
	updated, _, err := reconcileRootAgentFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	if updated {
		t.Error("current AGENTS.md must be left alone")
	}
	got, _ := os.ReadFile(layout.RootAgentsPath(root))
	if string(got) != current {
		t.Errorf("current AGENTS.md was overwritten: %q", got)
	}
}

func TestHintUnscaffoldedProjects_SkipsWorkspaceWorktree(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	app := filepath.Join(root, "app")
	runGit(t, root, "init", "app")
	runGit(t, app, "-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-m", "init")
	runGit(t, app, "worktree", "add", filepath.Join(root, "wt"))

	out := captureStdout(t, func() { hintUnscaffoldedProjects(root, nil) })
	if !strings.Contains(out, "app:") {
		t.Errorf("expected hint for app, got %q", out)
	}
	if strings.Contains(out, "wt:") {
		t.Errorf("unexpected hint for worktree: %q", out)
	}
}
