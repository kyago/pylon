package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kyago/pylon/internal/config"
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

	changed := syncEmbeddedDir(embeddedSkills, "skills", targetDir, ".md")

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
}

// TestSyncEmbeddedDir_SkipsUnchangedFile verifies that re-syncing identical
// content reports no changes (idempotent, no needless rewrites).
func TestSyncEmbeddedDir_SkipsUnchangedFile(t *testing.T) {
	targetDir := t.TempDir()

	first := syncEmbeddedDir(embeddedSkills, "skills", targetDir, ".md")
	if first == 0 {
		t.Fatal("expected files to be installed on first sync")
	}

	second := syncEmbeddedDir(embeddedSkills, "skills", targetDir, ".md")
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
