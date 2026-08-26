package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/layout"
)

// firstEmbeddedSkill returns the name of the first embedded skill .md file,
// skipping the test if none are present.
func firstEmbeddedSkill(t *testing.T) string {
	t.Helper()
	entries, err := embeddedSkills.ReadDir("skills")
	if err != nil {
		t.Fatalf("read embedded skills: %v", err)
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			return e.Name()
		}
	}
	t.Skip("no embedded skill files available to test")
	return ""
}

// TestSyncEmbeddedDir_RefreshesStaleFile reproduces the version-upgrade bug:
// an existing file with outdated content (from a previous pylon version) must
// be refreshed to the embedded content, not silently skipped.
func TestSyncEmbeddedDir_RefreshesStaleFile(t *testing.T) {
	targetDir := t.TempDir()
	name := firstEmbeddedSkill(t)

	want, err := embeddedSkills.ReadFile("skills/" + name)
	if err != nil {
		t.Fatalf("read embedded file: %v", err)
	}

	// Pre-seed a stale version, simulating a file installed by an older pylon.
	destPath := filepath.Join(targetDir, name)
	if err := os.WriteFile(destPath, []byte("STALE OLD VERSION\n"), 0644); err != nil {
		t.Fatal(err)
	}

	changed, refreshed := syncEmbeddedDir(embeddedSkills, "skills", targetDir, ".md")

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("stale file %s was not refreshed to embedded content", name)
	}
	if changed == 0 {
		t.Errorf("expected changed count > 0 when a stale file is refreshed, got 0")
	}
	// 덮어쓴 파일은 이름으로 보고되어야 한다 — 사용자가 수정한 파일일 수도 있다.
	if len(refreshed) != 1 || refreshed[0] != name {
		t.Errorf("expected refreshed = [%s], got %v", name, refreshed)
	}
}

// TestSyncEmbeddedDir_SkipsUnchangedFile verifies that re-syncing identical
// content reports no changes (idempotent, no needless rewrites).
func TestSyncEmbeddedDir_SkipsUnchangedFile(t *testing.T) {
	targetDir := t.TempDir()

	first, refreshed := syncEmbeddedDir(embeddedSkills, "skills", targetDir, ".md")
	if first == 0 {
		t.Fatal("expected files to be installed on first sync")
	}
	// 신규 설치는 '갱신'이 아니다 — 덮어쓴 것이 없으므로 보고 대상도 없다.
	if len(refreshed) != 0 {
		t.Errorf("expected no refreshed files on first install, got %v", refreshed)
	}

	second, _ := syncEmbeddedDir(embeddedSkills, "skills", targetDir, ".md")
	if second != 0 {
		t.Errorf("expected 0 changes on second sync of identical content, got %d", second)
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
	root := newTestWorkspace(t) // empty .pylon/commands -> embedded fallback

	desired := buildDesiredClaudeCommands(root)

	want := map[string]bool{
		filepath.Join("pl", "index.md"):        true,
		filepath.Join("pl", "project-list.md"): true,
	}
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

	// Apply, then a stale edit + a legacy file -> one changed + one removed.
	if err := applyClaudeCommands(commandsDir, desired); err != nil {
		t.Fatal(err)
	}
	staleTarget := filepath.Join("pl", "index.md")
	if err := os.WriteFile(filepath.Join(commandsDir, staleTarget), []byte("STALE\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(commandsDir, "status.md"), []byte("legacy\n"), 0644); err != nil {
		t.Fatal(err)
	}

	diff = diffClaudeCommands(commandsDir, desired)
	if len(diff.added) != 0 {
		t.Errorf("expected no additions, got %v", diff.added)
	}
	if len(diff.changed) != 1 || diff.changed[0] != staleTarget {
		t.Errorf("changed = %v, want [%s]", diff.changed, staleTarget)
	}
	if len(diff.removed) != 1 || diff.removed[0] != "status.md" {
		t.Errorf("removed = %v, want [status.md]", diff.removed)
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

	if _, err := os.Stat(filepath.Join(commandsDir, "pl", "index.md")); err != nil {
		t.Errorf("expected pl/index.md to be created after consent: %v", err)
	}
}

func TestSyncClaudeCommands_DeclineDoesNothing(t *testing.T) {
	root := newTestWorkspace(t)
	commandsDir := filepath.Join(root, ".claude", "commands")

	syncClaudeCommandsIfWorkspace(bytes.NewBufferString("n\n"), false)

	if _, err := os.Stat(filepath.Join(commandsDir, "pl", "index.md")); !os.IsNotExist(err) {
		t.Errorf("expected no commands written after decline, got err=%v", err)
	}
}

// TestSyncClaudeCommands_EmptyInputDeclines verifies default = No on EOF.
func TestSyncClaudeCommands_EmptyInputDeclines(t *testing.T) {
	root := newTestWorkspace(t)
	commandsDir := filepath.Join(root, ".claude", "commands")

	syncClaudeCommandsIfWorkspace(bytes.NewBufferString(""), false)

	if _, err := os.Stat(filepath.Join(commandsDir, "pl", "index.md")); !os.IsNotExist(err) {
		t.Errorf("expected no commands written on empty input, got err=%v", err)
	}
}

// TestSyncClaudeCommands_AutoYesSkipsPrompt applies without reading stdin.
func TestSyncClaudeCommands_AutoYesSkipsPrompt(t *testing.T) {
	root := newTestWorkspace(t)
	commandsDir := filepath.Join(root, ".claude", "commands")

	syncClaudeCommandsIfWorkspace(bytes.NewBufferString(""), true)

	if _, err := os.Stat(filepath.Join(commandsDir, "pl", "index.md")); err != nil {
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

func TestProviderDoctorCheckUsesConfiguredCommand(t *testing.T) {
	dir := t.TempDir()
	command := filepath.Join(dir, "custom-claude")
	if err := os.WriteFile(command, []byte("#!/bin/sh\necho custom-version\n"), 0755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Runtime: config.RuntimeConfig{Provider: "claude-code"},
		Providers: map[string]config.ProviderConfig{
			"claude-code": {Command: command},
		},
	}

	version, err := newProviderCheck(cfg, nil).Verify()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(version, "claude-code") || !strings.Contains(version, "custom-version") {
		t.Fatalf("provider version = %q", version)
	}
}

func TestProviderDoctorCheckDoesNotFallbackForExplicitUnknownProvider(t *testing.T) {
	cfg := &config.Config{Runtime: config.RuntimeConfig{Provider: "provider-b"}}
	_, err := newProviderCheck(cfg, nil).Verify()
	if err == nil || !strings.Contains(err.Error(), "provider-b") || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("provider check error = %v", err)
	}
}

func TestSyncConfigWarnsAboutDeprecatedBackend(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".pylon", "agents"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".pylon", "config.yml"), []byte(`version: "0.1"
runtime:
  backend: claude-code
`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".pylon", "agents", "legacy.md"), []byte(`---
name: legacy
role: Developer
backend: claude-code
---
Legacy agent.
`), 0644); err != nil {
		t.Fatal(err)
	}
	oldWorkspace := flagWorkspace
	flagWorkspace = root
	t.Cleanup(func() { flagWorkspace = oldWorkspace })

	output := captureStdout(t, syncConfigIfWorkspace)
	if !strings.Contains(output, "runtime.backend는 deprecated") || !strings.Contains(output, "runtime.provider: claude-code") {
		t.Fatalf("deprecated warning missing:\n%s", output)
	}
	if !strings.Contains(output, "agent legacy.md의 backend는 deprecated") || !strings.Contains(output, "provider: claude-code") {
		t.Fatalf("agent deprecated warning missing:\n%s", output)
	}
}

func TestUnknownProviderDoesNotMaterializeClaudeResources(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".pylon", "config.yml"), []byte(`version: "0.2"
runtime:
  provider: provider-b
`), 0644); err != nil {
		t.Fatal(err)
	}
	oldWorkspace := flagWorkspace
	flagWorkspace = root
	t.Cleanup(func() { flagWorkspace = oldWorkspace })

	output := captureStdout(t, func() {
		syncSelectedProviderResourcesIfWorkspace(bytes.NewBuffer(nil), true)
	})
	if !strings.Contains(output, "provider-b") || !strings.Contains(output, "건너뜀") {
		t.Fatalf("provider sync failure not reported:\n%s", output)
	}
	if _, err := os.Stat(filepath.Join(root, ".claude")); !os.IsNotExist(err) {
		t.Fatalf("unselected Claude resources were materialized: %v", err)
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

func TestSyncPylonResourcesRefreshesReference(t *testing.T) {
	pylonDir := t.TempDir()
	// 오래된 내용을 미리 심어 둔다 — 동기화가 임베디드 버전으로 되돌려야 한다.
	refDir := filepath.Join(pylonDir, "reference")
	if err := os.MkdirAll(refDir, 0755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(refDir, "pylon-usage.md")
	if err := os.WriteFile(stale, []byte("STALE"), 0644); err != nil {
		t.Fatal(err)
	}
	syncPylonResources(pylonDir)
	got, err := os.ReadFile(stale)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "pylon-usage-version:") {
		t.Errorf("reference not refreshed from embed, got: %.40q", got)
	}
}

func TestReconcileRootAgentFilesRebootstrapsStale(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(layout.PylonDir(root), 0755); err != nil {
		t.Fatal(err)
	}
	// 저버전 스탬프 = stale
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte("<!-- pylon-usage-version: 0 -->\n# old"), 0644); err != nil {
		t.Fatal(err)
	}
	bootstrapped, _, err := reconcileRootAgentFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	if !bootstrapped {
		t.Error("stale AGENTS.md should be re-bootstrapped")
	}
	got, _ := os.ReadFile(layout.RootAgentsPath(root))
	if !strings.Contains(string(got), "pylon-usage-version: 1") {
		t.Errorf("AGENTS.md not refreshed to current stamp: %.60q", got)
	}
}

func TestReconcileRootAgentFilesLeavesCurrent(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(layout.PylonDir(root), 0755); err != nil {
		t.Fatal(err)
	}
	authored := "<!-- pylon-usage-version: 1 -->\n# 저작됨"
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte(authored), 0644); err != nil {
		t.Fatal(err)
	}
	bootstrapped, _, err := reconcileRootAgentFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	if bootstrapped {
		t.Error("current AGENTS.md must be left alone")
	}
	got, _ := os.ReadFile(layout.RootAgentsPath(root))
	if string(got) != authored {
		t.Errorf("current AGENTS.md was overwritten: %q", got)
	}
}

func TestCheckProjectVerifyConfigs_RegeneratesMissing(t *testing.T) {
	requireGit(t)

	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".pylon", "config.yml"), []byte("version: \"0.1\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	projectDir := filepath.Join(tmpDir, "goproject")
	if out, err := exec.Command("git", "init", projectDir).CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module example.com/goproject\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// exclude 엔트리가 없는 상태에서 재생성이 일어나는 시나리오
	os.Remove(filepath.Join(projectDir, ".git", "info", "exclude"))
	// .pylon/ 없이 clone만 된 프로젝트는 재생성 대상이 아니어야 한다 (힌트만 출력)
	unscaffolded := filepath.Join(tmpDir, "freshclone")
	if err := os.MkdirAll(filepath.Join(unscaffolded, ".git"), 0755); err != nil {
		t.Fatal(err)
	}

	oldWorkspace := flagWorkspace
	flagWorkspace = tmpDir
	defer func() { flagWorkspace = oldWorkspace }()

	if ok := checkProjectVerifyConfigs(); !ok {
		t.Fatal("expected regeneration to succeed")
	}
	verifyPath := layout.VerifyConfigPath(projectDir)
	vc, err := config.LoadVerifyConfig(verifyPath)
	if err != nil {
		t.Fatalf("regenerated verify.yml must parse: %v", err)
	}
	if len(vc.OrderedSteps()) == 0 {
		t.Fatal("regenerated verify.yml for a Go project must contain commands")
	}
	// 재생성된 verify.yml이 커밋 가능해지면 안 된다 — exclude 엔트리가 함께 보장된다.
	if isGit, hasEntry := checkExcludeStatus(projectDir); !isGit || !hasEntry {
		t.Fatalf("regeneration must ensure .pylon/ exclude entry: isGit=%v hasEntry=%v", isGit, hasEntry)
	}
	// .pylon/ 없는 프로젝트에는 아무것도 쓰지 않는다.
	if _, err := os.Stat(filepath.Join(unscaffolded, ".pylon")); !os.IsNotExist(err) {
		t.Fatal("unscaffolded project must not be scaffolded by doctor")
	}
}

func TestCheckProjectVerifyConfigs_ReportsInvalidWithoutOverwriting(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".pylon", "config.yml"), []byte("version: \"0.1\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	projectDir := filepath.Join(tmpDir, "broken")
	if err := os.MkdirAll(filepath.Join(projectDir, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	invalid := "commands: [not: valid: yaml\n"
	verifyPath := layout.VerifyConfigPath(projectDir)
	if err := os.WriteFile(verifyPath, []byte(invalid), 0644); err != nil {
		t.Fatal(err)
	}

	oldWorkspace := flagWorkspace
	flagWorkspace = tmpDir
	defer func() { flagWorkspace = oldWorkspace }()

	if ok := checkProjectVerifyConfigs(); ok {
		t.Fatal("expected invalid verify.yml to fail the check")
	}
	data, err := os.ReadFile(verifyPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != invalid {
		t.Fatal("existing verify.yml must never be overwritten")
	}
}
