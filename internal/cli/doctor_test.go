package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
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
	wrote, removed, err := syncBuiltinResources(root)
	if err != nil {
		t.Fatal(err)
	}
	if !wrote || len(removed) != 0 {
		t.Fatalf("first sync: wrote=%v removed=%v, want true/none", wrote, removed)
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

	wrote, removed, err = syncBuiltinResources(root)
	if err != nil || wrote || len(removed) != 0 {
		t.Errorf("second sync: wrote=%v removed=%v err=%v, want false/none/nil", wrote, removed, err)
	}
}

// 이전 릴리스는 내장 리소스를 .pylon/<kind>/에 직접 복사했다. 내장 이름의 파일은 builtin을
// 가리지 않도록 비켜나야 한다: 내장과 같은 사본은 삭제, 내용이 다르면 .pylon-bak으로 보관.
// 같은 디렉토리의 다른 사용자 파일은 그대로 남아야 한다.
func TestSyncBuiltinResources_ClearsLegacyCopiesKeepsUserFiles(t *testing.T) {
	root := t.TempDir()
	files, err := embeddedBuiltinFiles()
	if err != nil {
		t.Fatal(err)
	}
	var agent, script builtinFile
	for _, f := range files {
		switch {
		case agent.rel == "" && strings.HasPrefix(f.rel, "agents/"):
			agent = f
		case script.rel == "" && strings.HasPrefix(f.rel, "scripts/bash/"):
			script = f
		}
	}
	seed := map[string][]byte{
		agent.rel:  agent.content,              // 내장과 동일한 사본 → 삭제
		script.rel: []byte("OLD RELEASE COPY"), // 내용이 다름 → 백업
	}
	for rel, content := range seed {
		path := filepath.Join(layout.PylonDir(root), filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	userAgent := filepath.Join(layout.ResourceDir(root, "agents"), "my-agent.md")
	if err := os.WriteFile(userAgent, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, cleared, err := syncBuiltinResources(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(cleared) != 2 {
		t.Errorf("cleared = %v, want 2 entries", cleared)
	}
	for rel := range seed {
		if _, err := os.Stat(filepath.Join(layout.PylonDir(root), filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Errorf("legacy copy .pylon/%s survived", rel)
		}
	}
	scriptPath := filepath.Join(layout.PylonDir(root), filepath.FromSlash(script.rel))
	if got, err := os.ReadFile(scriptPath + rootFileBackupSuffix); err != nil || string(got) != "OLD RELEASE COPY" {
		t.Errorf("differing copy must be backed up: %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(layout.PylonDir(root), filepath.FromSlash(agent.rel)) + rootFileBackupSuffix); !os.IsNotExist(err) {
		t.Error("identical copy must be deleted, not backed up")
	}
	if got, err := os.ReadFile(userAgent); err != nil || string(got) != "mine" {
		t.Errorf("user agent touched: %q, %v", got, err)
	}
	if _, err := os.Stat(layout.ResourceDir(root, "reference")); !os.IsNotExist(err) {
		t.Error("emptied built-in-only dir .pylon/reference should be removed")
	}
}

// 2026.9.2까지 .pylon/<kind>/에 직접 복사되던 내장 중 이제 은퇴한 리소스: 출하 내용 그대로면 삭제,
// 편집됐으면 .pylon-bak으로 보관하고, 목록에 없는 사용자 에이전트는 남긴다. 남은 링크는 기존
// prune으로 .claude/agents/에서 사라진다.
func TestSyncBuiltinResources_ClearsRetiredBuiltins(t *testing.T) {
	root := t.TempDir()
	shipped := []byte("retired built-in as shipped")
	for _, rel := range []string{"agents/retired-agent.md", "skills/retired-skill.md"} {
		addLegacyShipped(t, rel, shipped)
	}
	agent := filepath.Join(layout.ResourceDir(root, "agents"), "retired-agent.md")
	skill := filepath.Join(layout.ResourceDir(root, "skills"), "retired-skill.md")
	userAgent := filepath.Join(layout.ResourceDir(root, "agents"), "my-agent.md")
	for path, content := range map[string][]byte{agent: shipped, skill: []byte("edited by user"), userAgent: []byte("mine")} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	claudeAgents := layout.ClaudeAgentsDir(root)
	if err := os.MkdirAll(claudeAgents, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(claudeAgents, "retired-agent.md")
	if err := os.Symlink(layout.AgentLinkTarget("retired-agent.md"), link); err != nil {
		t.Fatal(err)
	}

	_, cleared, err := syncBuiltinResources(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(cleared) != 2 {
		t.Errorf("cleared = %v, want 2 entries", cleared)
	}
	if _, err := os.Lstat(agent); !os.IsNotExist(err) {
		t.Error("unchanged retired agent must be deleted")
	}
	if _, err := os.Lstat(agent + rootFileBackupSuffix); !os.IsNotExist(err) {
		t.Error("unchanged retired agent must not be backed up")
	}
	if _, err := os.Lstat(skill); !os.IsNotExist(err) {
		t.Error("edited retired skill must be moved aside")
	}
	if got, err := os.ReadFile(skill + rootFileBackupSuffix); err != nil || string(got) != "edited by user" {
		t.Errorf("edited retired skill must be backed up: %q, %v", got, err)
	}
	if got, err := os.ReadFile(userAgent); err != nil || string(got) != "mine" {
		t.Errorf("user agent touched: %q, %v", got, err)
	}

	cfg, err := config.ParseConfig([]byte("version: \"0.1\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err := generateClaudeAgentsWithSkills(root, cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Error(".claude/agents/ link to the retired agent must be pruned")
	}
	if _, err := os.Lstat(filepath.Join(claudeAgents, "my-agent.md")); err != nil {
		t.Errorf("user agent link missing: %v", err)
	}
}

// addLegacyShipped registers content as a previously shipped version of rel for one test.
func addLegacyShipped(t *testing.T, rel string, content []byte) {
	t.Helper()
	sum := sha256.Sum256(content)
	hash := hex.EncodeToString(sum[:])
	if legacyShipped[rel] == nil {
		legacyShipped[rel] = map[string]bool{}
		t.Cleanup(func() { delete(legacyShipped, rel) })
	} else {
		t.Cleanup(func() { delete(legacyShipped[rel], hash) })
	}
	legacyShipped[rel][hash] = true
}

// 2026.9.2 이전 레이아웃은 git 추적 .pylon/<kind>/에 내장 사본을 두었다. 이번 릴리스가 내용을 바꾼
// 내장이라도, 손대지 않은 과거 출하본이면 백업하지 않고 삭제해야 한다.
func TestSyncBuiltinResources_DeletesLegacyShippedCopiesOfCurrentBuiltins(t *testing.T) {
	root := t.TempDir()
	oldAgent := []byte("architect as shipped by 2026.9.2")
	oldCommand := []byte("pl-pipeline as shipped by 2026.9.2")
	addLegacyShipped(t, "agents/architect.md", oldAgent)
	addLegacyShipped(t, "commands/pl-pipeline.md", oldCommand)
	files := map[string][]byte{
		filepath.Join(layout.ResourceDir(root, "agents"), "architect.md"):     oldAgent,
		filepath.Join(layout.ResourceDir(root, "commands"), "pl-pipeline.md"): oldCommand,
		filepath.Join(layout.ResourceDir(root, "agents"), "critic.md"):        []byte("critic edited by user"),
	}
	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if _, _, err := syncBuiltinResources(root); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"agents/architect.md", "commands/pl-pipeline.md"} {
		path := filepath.Join(layout.ResourceDir(root, filepath.Dir(rel)), filepath.Base(rel))
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Errorf("%s: unedited legacy copy must be deleted", rel)
		}
		if _, err := os.Lstat(path + rootFileBackupSuffix); !os.IsNotExist(err) {
			t.Errorf("%s: unedited legacy copy must not be backed up", rel)
		}
	}
	critic := filepath.Join(layout.ResourceDir(root, "agents"), "critic.md")
	if got, err := os.ReadFile(critic + rootFileBackupSuffix); err != nil || string(got) != "critic edited by user" {
		t.Errorf("edited copy must be backed up: %q, %v", got, err)
	}
}

// 은퇴 목록은 과거 출하 기록 중 현재 임베드에 없는 경로다 — 이번 릴리스가 지운 에이전트 20개와 스킬 2개.
func TestRetiredBuiltins_AreLegacyPathsMissingFromEmbeds(t *testing.T) {
	got, err := retiredBuiltins()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"agents/academic-analyst.md", "agents/backend-dev.md", "agents/brand-strategist.md",
		"agents/campaign-planner.md", "agents/code-simplifier.md", "agents/content-reviewer.md",
		"agents/content-strategist.md", "agents/copywriter.md", "agents/data-analyst.md",
		"agents/doc-specialist.md", "agents/frontend-dev.md", "agents/lead-researcher.md",
		"agents/pm.md", "agents/po.md", "agents/report-writer.md", "agents/security-reviewer.md",
		"agents/tech-writer.md", "agents/test-engineer.md", "agents/tracer.md", "agents/web-searcher.md",
		"commands/pl-cancel.md", "skills/content-writing-guide.md", "skills/marketing-framework.md",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("retiredBuiltins() =\n%v\nwant\n%v", got, want)
	}
	for rel, hashes := range legacyShipped {
		if len(hashes) == 0 {
			t.Errorf("%s: no shipped hash", rel)
		}
		for h := range hashes {
			if len(h) != 64 {
				t.Errorf("%s: malformed hash %q", rel, h)
			}
		}
	}
}

// pylon doctor/update도 .claude/settings.json 훅을 현재 hooks.json으로 갱신한다 — 구버전 Stop 훅이
// `claude`를 직접 띄운 세션에서 매 턴 실패하지 않도록. 사용자 훅은 남는다.
func TestSyncClaudeProviderResources_RefreshesSettingsHooks(t *testing.T) {
	root := setupTestWorkspace(t)
	claudeDir := layout.ClaudeDir(root)
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := `{"hooks":{
  "Stop":[{"matcher":"*","hooks":[{"type":"command","command":"pylon sync-memory --from-session --agent claude"}]}],
  "PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"my-guard"}]}]
}}`
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	oldWorkspace := flagWorkspace
	flagWorkspace = root
	t.Cleanup(func() { flagWorkspace = oldWorkspace })

	captureStdout(t, func() { syncClaudeProviderResourcesIfWorkspace(bytes.NewBuffer(nil), true) })

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
	if _, ok := settings.Hooks["Stop"]; ok {
		t.Errorf("legacy pylon Stop hook must be removed: %s", data)
	}
	if len(settings.Hooks["SessionStart"]) != 1 {
		t.Errorf("SessionStart hook missing: %s", data)
	}
	if pre := settings.Hooks["PreToolUse"]; len(pre) != 1 || pre[0].Hooks[0].Command != "my-guard" {
		t.Errorf("user hook must be kept: %s", data)
	}
}

// builtin은 매번 파일 단위로 맞춘다: 지워지거나 고쳐진 파일은 복구하고, 내장에 없는 파일은
// 지운다(바이너리에서 빠진 리소스). 이미 맞으면 아무것도 바꾸지 않는다.
func TestSyncBuiltinResources_RepairsAndPrunesBuiltin(t *testing.T) {
	root := t.TempDir()
	if _, _, err := syncBuiltinResources(root); err != nil {
		t.Fatal(err)
	}
	common := filepath.Join(layout.ScriptsDir(root), "common.sh")
	builtinAgent := filepath.Join(layout.BuiltinResourceDir(root, "agents"), "architect.md")
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

	changed, _, err := syncBuiltinResources(root)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Error("a damaged builtin must be reported as changed")
	}
	if info, err := os.Stat(common); err != nil || info.Mode().Perm()&0o100 == 0 {
		t.Errorf("deleted script not restored as executable: %v", err)
	}
	want, _ := embeddedAgents.ReadFile("agents/architect.md")
	if got, _ := os.ReadFile(builtinAgent); !bytes.Equal(got, want) {
		t.Error("edited builtin file not restored")
	}
	if _, err := os.Stat(dropped); !os.IsNotExist(err) {
		t.Error("resource absent from the embeds survived")
	}
	if changed, _, err := syncBuiltinResources(root); err != nil || changed {
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

// 사용자 커맨드(.pylon/commands/)는 추가되지만, 내장 이름과 겹치면 내장 내용이 이긴다.
func TestBuildDesiredClaudeCommands_UserCommands(t *testing.T) {
	root := newTestWorkspace(t)
	userDir := layout.ResourceDir(root, "commands")
	for name, body := range map[string]string{"pl-mine.md": "# mine\n", "pl-verify.md": "# shadow\n"} {
		if err := os.WriteFile(filepath.Join(userDir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	desired := buildDesiredClaudeCommands(root)
	if desired[filepath.Join("pl", "mine.md")] != "# mine\n" {
		t.Error("user command missing from desired set")
	}
	want, _ := embeddedCommands.ReadFile("commands/pl-verify.md")
	if desired[filepath.Join("pl", "verify.md")] != string(want) {
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
	// config의 backend는 경고 대신 자동 마이그레이션된다.
	if !strings.Contains(output, "runtime.backend를 runtime.provider로 전환했습니다") {
		t.Fatalf("migration notice missing:\n%s", output)
	}
	if strings.Contains(output, "runtime.backend는 deprecated") {
		t.Fatalf("deprecated warning must not appear after migration:\n%s", output)
	}
	cfg, err := config.LoadConfig(filepath.Join(root, ".pylon", "config.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if providerName, deprecated := cfg.Runtime.EffectiveProvider(); providerName != "claude-code" || deprecated {
		t.Fatalf("config not migrated: provider=%q deprecated=%v", providerName, deprecated)
	}
	// agent frontmatter는 사용자 소유라 자동 rewrite하지 않고 경고만 유지한다.
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

func TestRuntimeEnvCheckWarnsOnOverridingKeys(t *testing.T) {
	const suffix = "는 세션·에이전트별 effort/compact 설정을 모두 덮어씁니다. 이 키를 제거해야 runtime.effort가 적용됩니다"
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"nil env", nil, ""},
		{"unrelated key", map[string]string{"FOO": "1"}, ""},
		{"effort key", map[string]string{"CLAUDE_CODE_EFFORT_LEVEL": "high"}, "runtime.env의 CLAUDE_CODE_EFFORT_LEVEL" + suffix},
		{"both keys", map[string]string{"CLAUDE_AUTOCOMPACT_PCT_OVERRIDE": "80", "CLAUDE_CODE_EFFORT_LEVEL": "high"},
			"runtime.env의 CLAUDE_CODE_EFFORT_LEVEL, CLAUDE_AUTOCOMPACT_PCT_OVERRIDE" + suffix},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check := newRuntimeEnvCheck(&config.Config{Runtime: config.RuntimeConfig{Env: tt.env}})
			_, err := check.Verify()
			got := ""
			if err != nil {
				got = err.Error()
			}
			if got != tt.want {
				t.Fatalf("Verify() error = %q, want %q", got, tt.want)
			}
		})
	}
	// 설정 로드 실패(cfg nil)는 provider 체크가 보고하므로 여기서는 통과시킨다.
	if _, err := newRuntimeEnvCheck(nil).Verify(); err != nil {
		t.Fatalf("nil config must not warn: %v", err)
	}
}

func TestSyncConfigRemovesLegacyRuntimeEnvDefaults(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(root, ".pylon", "config.yml")
	if err := os.WriteFile(cfgPath, []byte(`version: "0.2"
runtime:
  provider: claude-code
  env:
    CLAUDE_AUTOCOMPACT_PCT_OVERRIDE: "80"
    CLAUDE_CODE_EFFORT_LEVEL: high
`), 0644); err != nil {
		t.Fatal(err)
	}
	oldWorkspace := flagWorkspace
	flagWorkspace = root
	t.Cleanup(func() { flagWorkspace = oldWorkspace })

	output := captureStdout(t, syncConfigIfWorkspace)
	for _, key := range []string{"CLAUDE_CODE_EFFORT_LEVEL", "CLAUDE_AUTOCOMPACT_PCT_OVERRIDE"} {
		if !strings.Contains(output, "✓ runtime.env."+key+" 제거 (pylon 구버전 기본값") {
			t.Fatalf("removal notice for %s missing:\n%s", key, output)
		}
	}
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Runtime.Env != nil {
		t.Fatalf("legacy env must be removed, got %v", cfg.Runtime.Env)
	}
}

func TestRunChecksTreatsWarnCheckAsNonFailure(t *testing.T) {
	var allPassed bool
	var failures []Check
	output := captureStdout(t, func() {
		allPassed, failures = runChecks([]Check{
			newRuntimeEnvCheck(&config.Config{Runtime: config.RuntimeConfig{Env: map[string]string{"CLAUDE_CODE_EFFORT_LEVEL": "high"}}}),
		})
	})
	if !allPassed || len(failures) != 0 {
		t.Fatalf("warn check must not fail doctor: allPassed=%v failures=%v", allPassed, failures)
	}
	if !strings.Contains(output, "⚠ runtime.env") || !strings.Contains(output, "CLAUDE_CODE_EFFORT_LEVEL는") {
		t.Fatalf("warning not printed:\n%s", output)
	}
}

func TestReconcileRootAgentFilesReplacesLegacyGuide(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(layout.PylonDir(root), 0755); err != nil {
		t.Fatal(err)
	}
	// 블록 도입 이전의 스탬프 전체 파일 = 레거시 가이드
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte("<!-- pylon-usage-version: 0 -->\n# old"), 0644); err != nil {
		t.Fatal(err)
	}
	updated, _, err := reconcileRootAgentFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	if !updated {
		t.Error("legacy AGENTS.md should be replaced")
	}
	got, _ := os.ReadFile(layout.RootAgentsPath(root))
	if string(got) != buildAgentsBlock(nil) {
		t.Errorf("AGENTS.md not refreshed to the generated block: %.60q", got)
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

func TestCheckProjectVerifyConfigs_SingleRepoRootIsCovered(t *testing.T) {
	requireGit(t)

	tmpDir := t.TempDir()
	if out, err := exec.Command("git", "init", tmpDir).CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".pylon", "config.yml"), []byte("version: \"0.1\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module example.com/single\n"), 0644); err != nil {
		t.Fatal(err)
	}

	oldWorkspace := flagWorkspace
	flagWorkspace = tmpDir
	defer func() { flagWorkspace = oldWorkspace }()

	if ok := checkProjectVerifyConfigs(); !ok {
		t.Fatal("single-repo root regeneration must succeed")
	}
	vc, err := config.LoadVerifyConfig(layout.VerifyConfigPath(tmpDir))
	if err != nil {
		t.Fatalf("root verify.yml must be regenerated and parseable: %v", err)
	}
	if len(vc.OrderedSteps()) == 0 {
		t.Fatal("root verify.yml for a Go repo must contain commands")
	}
	// 루트 repo에서는 .pylon/ 전체가 아니라 verify.yml만 exclude된다 —
	// 워크스페이스 .pylon/은 git-tracked 대상이다.
	exclude, err := os.ReadFile(filepath.Join(tmpDir, ".git", "info", "exclude"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(exclude), ".pylon/verify.yml") {
		t.Fatalf("root exclude must contain .pylon/verify.yml:\n%s", exclude)
	}
	for _, line := range strings.Split(string(exclude), "\n") {
		if strings.TrimSpace(line) == ".pylon/" {
			t.Fatalf("root exclude must not hide the whole .pylon/:\n%s", exclude)
		}
	}
}

func TestCheckProjectVerifyConfigs_RootSkippedWhenSubprojectsExist(t *testing.T) {
	requireGit(t)

	tmpDir := t.TempDir()
	if out, err := exec.Command("git", "init", tmpDir).CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".pylon", "config.yml"), []byte("version: \"0.1\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	projectDir := filepath.Join(tmpDir, "svc")
	if err := os.MkdirAll(filepath.Join(projectDir, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module example.com/svc\n"), 0644); err != nil {
		t.Fatal(err)
	}

	oldWorkspace := flagWorkspace
	flagWorkspace = tmpDir
	defer func() { flagWorkspace = oldWorkspace }()

	if ok := checkProjectVerifyConfigs(); !ok {
		t.Fatal("subproject regeneration must succeed")
	}
	// 하위 프로젝트가 있으면 루트는 검증 대상이 아니다 — 비어 있는 루트
	// verify.yml은 GIT_ROOT 오해석 가드를 무력화한다.
	if _, err := os.Stat(layout.VerifyConfigPath(tmpDir)); !os.IsNotExist(err) {
		t.Fatal("workspace root must not get a verify.yml when subprojects exist")
	}
	if _, err := os.Stat(layout.VerifyConfigPath(projectDir)); err != nil {
		t.Fatalf("subproject verify.yml must be regenerated: %v", err)
	}
}

func TestCheckProjectVerifyConfigs_UnknownStackRootIsSkipped(t *testing.T) {
	requireGit(t)

	tmpDir := t.TempDir()
	if out, err := exec.Command("git", "init", tmpDir).CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".pylon", "config.yml"), []byte("version: \"0.1\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	oldWorkspace := flagWorkspace
	flagWorkspace = tmpDir
	defer func() { flagWorkspace = oldWorkspace }()

	if ok := checkProjectVerifyConfigs(); !ok {
		t.Fatal("workspace-holder root must not fail the check")
	}
	// 스택 미감지 루트에 no-op verify.yml을 만들면 GIT_ROOT 가드가 무력화된다.
	if _, err := os.Stat(layout.VerifyConfigPath(tmpDir)); !os.IsNotExist(err) {
		t.Fatal("unknown-stack root must not get a verify.yml")
	}
}
