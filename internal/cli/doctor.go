package cli

import (
	"bufio"
	"bytes"
	"context"
	"embed"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/layout"
	"github.com/kyago/pylon/internal/provider"
	"github.com/spf13/cobra"
)

// Check represents a single dependency check.
// Spec Reference: Section 7 "pylon doctor"
type Check struct {
	Name        string
	Required    bool
	Verify      func() (version string, err error)
	InstallURL  string
	BrewFormula string // Homebrew formula name; empty when brew install is not documented
}

var baseChecks = []Check{
	{
		Name:       "git",
		Required:   true,
		Verify:     verifyGit,
		InstallURL: "https://git-scm.com/downloads",
	},
	{
		Name:       "gh",
		Required:   true,
		Verify:     verifyGH,
		InstallURL: "https://cli.github.com/",
	},
}

func newDoctorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check required tool installations and versions",
		Long: `Verify that required tools and the selected runtime provider are installed and configured.

Also refreshes the pylon-owned resources under .pylon/ (agents, skills, commands,
scripts) to the versions shipped with this binary. Files you added yourself — a new
agent, a new skill, anything whose name is not shipped with pylon — are left alone.

Use --fix-excludes to automatically add .pylon/ to project .git/info/exclude
for any projects that are missing the local-scope ignore entry.`,
		RunE: runDoctor,
	}

	cmd.Flags().Bool("fix-excludes", false, "auto-fix missing .pylon/ exclude entries in project repos")
	cmd.Flags().Bool("yes", false, "커맨드 동기화 확인 프롬프트를 건너뛰고 자동 승인")

	return cmd
}

// runChecks executes all doctor checks and returns results.
func runChecks(checks []Check) (allPassed bool, failures []Check) {
	allPassed = true
	for _, check := range checks {
		ver, err := check.Verify()
		reqLabel := ""
		if check.Required {
			reqLabel = " [required]"
		}
		if err != nil {
			allPassed = false
			failures = append(failures, check)
			fmt.Printf("\u2717 %-10s %-10s%s\n", check.Name, "missing", reqLabel)
		} else {
			fmt.Printf("\u2713 %-10s %-10s%s\n", check.Name, ver, reqLabel)
		}
	}
	return allPassed, failures
}

func runDoctor(cmd *cobra.Command, args []string) error {
	fmt.Println("Pylon Doctor")
	fmt.Println(strings.Repeat("\u2500", 40))

	allPassed, failures := runChecks(currentDoctorChecks())

	// Sync config defaults if in a workspace
	fmt.Println()
	syncConfigIfWorkspace()

	// Sync provider-neutral embedded resources if in a workspace.
	syncResourcesIfWorkspace()

	// Reconcile resources owned by the selected provider.
	autoYes, _ := cmd.Flags().GetBool("yes")
	syncSelectedProviderResourcesIfWorkspace(cmd.InOrStdin(), autoYes)

	// Check project repo .pylon/ exclude settings (skip if git is missing)
	fixExcludes, _ := cmd.Flags().GetBool("fix-excludes")
	hasGit := true
	for _, f := range failures {
		if f.Name == "git" {
			hasGit = false
			break
		}
	}
	if hasGit {
		if !checkRepoExcludes(fixExcludes) {
			allPassed = false
		}
	} else {
		fmt.Println()
		fmt.Println("⚠ git 미설치로 프로젝트 exclude 검사 건너뜀")
	}

	if !checkProjectVerifyConfigs() {
		allPassed = false
	}

	fmt.Println()
	if allPassed {
		fmt.Println("All checks passed.")
		return nil
	}

	printInstallHints(failures)
	return fmt.Errorf("doctor checks failed")
}

// installHint returns the install guidance for a failed check.
// When the tool has a brew formula and brew is available, the brew
// command is shown first with the download URL as fallback.
func installHint(c Check, hasBrew bool) string {
	if c.BrewFormula != "" && hasBrew {
		return fmt.Sprintf("brew install %s (또는 %s)", c.BrewFormula, c.InstallURL)
	}
	return c.InstallURL
}

// printInstallHints prints install guidance for each failed check.
func printInstallHints(failures []Check) {
	_, err := exec.LookPath("brew")
	hasBrew := err == nil
	fmt.Println("Some checks failed. Install missing tools:")
	for _, f := range failures {
		fmt.Printf("  %s: %s\n", f.Name, installHint(f, hasBrew))
	}
}

// checkRepoExcludes verifies that all projects have .pylon/
// in their .git/info/exclude for local-scope ignore.
// When fix is true, missing entries are automatically added.
// Returns true if all projects are OK, false if any are missing.
func checkRepoExcludes(fix bool) bool {
	root, err := resolveRoot()
	if err != nil {
		return true // not in a workspace, nothing to check
	}

	projects, err := config.DiscoverProjects(root)
	if err != nil || len(projects) == 0 {
		return true
	}

	// Filter to git-only projects and check exclude entries
	var gitProjects []config.ProjectInfo
	var missing []config.ProjectInfo
	for _, p := range projects {
		isGit, hasEntry := checkExcludeStatus(p.Path)
		if !isGit {
			continue // skip non-git directories
		}
		gitProjects = append(gitProjects, p)
		if !hasEntry {
			missing = append(missing, p)
		}
	}

	if len(gitProjects) == 0 {
		return true
	}

	fmt.Println()
	if len(missing) == 0 {
		fmt.Printf("✓ 모든 프로젝트에 .pylon/ exclude 설정됨 (%d개 프로젝트)\n", len(gitProjects))
		return true
	}

	if fix {
		fixed := 0
		for _, p := range missing {
			if err := excludePylonFromRepo(p.Path); err != nil {
				fmt.Printf("⚠ %s: exclude 설정 실패: %v\n", p.Name, err)
			} else {
				fmt.Printf("✓ %s: .pylon/ exclude 추가됨\n", p.Name)
				fixed++
			}
		}
		if fixed == len(missing) {
			fmt.Printf("✓ %d개 프로젝트 exclude 수정 완료\n", fixed)
			return true
		}
		fmt.Printf("⚠ %d/%d 프로젝트 exclude 수정 완료, %d개 실패\n", fixed, len(missing), len(missing)-fixed)
		return false
	}

	fmt.Printf("⚠ .pylon/ exclude 미설정 프로젝트 %d개:\n", len(missing))
	for _, p := range missing {
		fmt.Printf("  - %s\n", p.Name)
	}
	fmt.Println("  수정: pylon doctor --fix-excludes 또는 각 프로젝트의 .git/info/exclude에 '.pylon/' 추가")
	return false
}

// checkProjectVerifyConfigs ensures every discovered project has a usable
// .pylon/verify.yml. verify.yml is local-only state (excluded from the project
// repo by design), so it is lost outside this machine and nothing else recreates
// it — a missing file is regenerated from the detected tech stack, the same way
// `pylon add-project` scaffolds it, and the .pylon/ exclude entry is re-applied
// so the regenerated file can never become committable. Discovery only sees
// directories that still have a .pylon/ dir; a fully fresh clone (no .pylon/ at
// all) is reported as a hint to run `pylon add-project --skip-clone` instead.
// An existing file is user data and is never overwritten: parse failures and
// empty command sets are only reported.
// Returns true when every project ends up with a parseable, non-empty config.
func checkProjectVerifyConfigs() bool {
	root, err := resolveRoot()
	if err != nil {
		return true // not in a workspace, nothing to check
	}
	projects, err := config.DiscoverProjects(root)
	if err != nil {
		return true
	}

	fmt.Println()
	hintUnscaffoldedProjects(root, projects)
	if len(projects) == 0 {
		return true
	}

	ok := true
	issues := 0
	for _, p := range projects {
		path := layout.VerifyConfigPath(p.Path)
		if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
			content := generateVerifyYML(detectTechStack(p.Path))
			if writeErr := os.WriteFile(path, []byte(content), 0o644); writeErr != nil {
				fmt.Printf("⚠ %s: verify.yml 재생성 실패: %v\n", p.Name, writeErr)
				ok = false
				issues++
				continue
			}
			fmt.Printf("✓ %s: 누락된 verify.yml 재생성됨\n", p.Name)
			// 재생성한 파일이 프로젝트 repo에 커밋 가능해지지 않도록
			// add-project와 동일하게 exclude 엔트리를 보장한다.
			if isGit, hasEntry := checkExcludeStatus(p.Path); isGit && !hasEntry {
				if exclErr := excludePylonFromRepo(p.Path); exclErr != nil {
					fmt.Printf("⚠ %s: .pylon/ exclude 설정 실패: %v\n", p.Name, exclErr)
					ok = false
					issues++
				}
			}
		} else if statErr != nil {
			fmt.Printf("⚠ %s: verify.yml 확인 실패: %v\n", p.Name, statErr)
			ok = false
			issues++
			continue
		}

		vc, loadErr := config.LoadVerifyConfig(path)
		if loadErr != nil {
			fmt.Printf("⚠ %s: verify.yml 파싱 실패 — 직접 수정이 필요합니다: %v\n", p.Name, loadErr)
			ok = false
			issues++
			continue
		}
		if len(vc.OrderedSteps())+len(vc.OrderedHeldOutSteps()) == 0 {
			fmt.Printf("⚠ %s: verify.yml에 실행 가능한 검증 명령이 없습니다: %s\n", p.Name, path)
			ok = false
			issues++
		}
	}
	if issues == 0 {
		fmt.Printf("✓ 모든 프로젝트 verify.yml 정상 (%d개 프로젝트)\n", len(projects))
	}
	return ok
}

// hintUnscaffoldedProjects points out workspace-root git repos that have no
// .pylon/ at all (e.g. a fresh clone) — doctor cannot regenerate verify.yml
// for them because project discovery requires .pylon/ to exist.
func hintUnscaffoldedProjects(root string, projects []config.ProjectInfo) {
	known := make(map[string]bool, len(projects))
	for _, p := range projects {
		known[p.Name] = true
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || strings.HasPrefix(name, ".") || known[name] {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, name, ".git")); err != nil {
			continue
		}
		fmt.Printf("ℹ %s: .pylon/ 미초기화 git 프로젝트 — pylon add-project %s --skip-clone 으로 초기화하세요\n", name, name)
	}
}

// checkExcludeStatus returns whether a project is a git repo and whether
// its .git/info/exclude contains .pylon/.
func checkExcludeStatus(projectDir string) (isGitRepo bool, hasEntry bool) {
	excludePath, err := resolveGitExcludePath(projectDir)
	if err != nil {
		return false, false // not a git repo
	}

	data, err := os.ReadFile(excludePath)
	if err != nil {
		return true, false // git repo but no exclude file
	}

	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == ".pylon/" {
			return true, true
		}
	}
	return true, false
}

// RunDoctorChecks runs doctor checks with detailed output and returns whether all passed.
// Used internally by pylon init.
func RunDoctorChecks() (bool, error) {
	fmt.Println("Pylon Doctor")
	fmt.Println(strings.Repeat("\u2500", 40))

	allPassed, failures := runChecks(currentDoctorChecks())

	fmt.Println()
	if !allPassed {
		printInstallHints(failures)
	}

	return allPassed, nil
}

func currentDoctorChecks() []Check {
	checks := append([]Check(nil), baseChecks...)
	cfg, err := doctorProviderConfig()
	return append(checks, newProviderCheck(cfg, err))
}

func doctorProviderConfig() (*config.Config, error) {
	root, err := resolveRoot()
	if err != nil {
		return config.ParseConfig([]byte("version: \"0.2\"\n"))
	}
	return config.LoadConfig(layout.ConfigPath(root))
}

func newProviderCheck(cfg *config.Config, configErr error) Check {
	installURL := claudeInstallURL
	if cfg != nil {
		providerName, _ := cfg.Runtime.EffectiveProvider()
		if catalog, err := newProviderCatalog(cfg); err == nil {
			installURL = catalog.installURL(providerName)
		}
	}

	return Check{
		Name:       "provider",
		Required:   true,
		InstallURL: installURL,
		Verify: func() (string, error) {
			if configErr != nil {
				return "", fmt.Errorf("provider config load failed: %w", configErr)
			}
			catalog, err := newProviderCatalog(cfg)
			if err != nil {
				return "", err
			}
			_, selection, err := catalog.selectInteractive(context.Background(), cfg)
			if err != nil {
				return "", err
			}
			versioned, ok := selection.Adapter.(provider.VersionedAdapter)
			if !ok {
				return selection.Adapter.Name(), nil
			}
			version, err := versioned.Version(context.Background())
			if err != nil {
				return "", err
			}
			return selection.Adapter.Name() + " " + version, nil
		},
	}
}

func verifyGit() (string, error) {
	out, err := exec.Command("git", "--version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git not found: %w", err)
	}
	// "git version 2.44.0"
	ver := strings.TrimSpace(string(out))
	ver = strings.TrimPrefix(ver, "git version ")
	return ver, nil
}

func verifyGH() (string, error) {
	out, err := exec.Command("gh", "--version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("gh not found: %w", err)
	}
	// "gh version 2.65.0 (2024-...)"
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 {
		return "", fmt.Errorf("gh version output empty")
	}
	parts := strings.Fields(lines[0])
	if len(parts) >= 3 {
		return parts[2], nil
	}
	return lines[0], nil
}

// syncConfigIfWorkspace syncs config.yml defaults if running inside a pylon workspace.
func syncConfigIfWorkspace() {
	root, err := resolveRoot()
	if err != nil {
		return // not in a workspace, skip
	}

	cfgPath := layout.ConfigPath(root)
	cfg, added, err := config.SyncConfigDefaults(cfgPath)
	if err != nil {
		fmt.Printf("⚠ 설정 동기화 실패: %v\n", err)
		return
	}
	if len(added) > 0 {
		fmt.Println("✓ config.yml에 누락된 기본값 추가:")
		for _, field := range added {
			fmt.Printf("  + %s\n", field)
		}
	} else {
		fmt.Println("✓ config.yml 최신 상태")
	}
	if providerName, deprecated := cfg.Runtime.EffectiveProvider(); deprecated {
		fmt.Printf("⚠ runtime.backend는 deprecated입니다 — runtime.provider: %s 로 전환하세요 (자동 rewrite하지 않음)\n", providerName)
	}
	warnDeprecatedAgentProviders(root)
}

func warnDeprecatedAgentProviders(root string) {
	agentPaths, err := filepath.Glob(filepath.Join(layout.PylonDir(root), "agents", "*.md"))
	if err != nil {
		return
	}
	for _, path := range agentPaths {
		agent, err := config.ParseAgentFile(path)
		if err != nil {
			continue
		}
		providerName, deprecated := agent.EffectiveProvider()
		if !deprecated {
			continue
		}
		fmt.Printf("⚠ agent %s의 backend는 deprecated입니다 — provider: %s 로 전환하세요 (자동 rewrite하지 않음)\n", filepath.Base(path), providerName)
	}
}

// reconcileRootAgentFiles refreshes the CLAUDE.md marker and re-bootstraps AGENTS.md
// when it is missing or older than the embedded manual, so `pylon doctor` recovers a
// workspace whose root guide drifted. It never invokes an LLM — the next launched
// session authors the refreshed bootstrap. Returns whether AGENTS.md was bootstrapped
// and the names of any hand-written root files moved aside first.
func reconcileRootAgentFiles(root string) (bool, []string, error) {
	projects, err := config.DiscoverProjects(root)
	if err != nil {
		projects = nil // 탐색 실패 시 팩트 없는 부트스트랩이라도 최신화한다
	}
	return ensureRootAgentFiles(root, projects)
}

// syncResourcesIfWorkspace syncs provider-neutral embedded agents, skills, commands, and scripts
// to the workspace if running inside a pylon workspace.
// .pylon/ resources are pylon-managed: missing files are installed and files
// whose content differs from the embedded version are refreshed, so upgrades
// pick up new resource content.
func syncResourcesIfWorkspace() {
	root, err := resolveRoot()
	if err != nil {
		return // not in a workspace, skip
	}

	pylonDir := layout.PylonDir(root)
	totalWritten, overwritten := syncPylonResources(pylonDir)

	if totalWritten > 0 {
		fmt.Printf("✓ 내장 리소스 %d개 설치/갱신\n", totalWritten)
	} else {
		fmt.Println("✓ 내장 리소스 최신 상태")
	}
	// pylon 소유 파일을 내장 버전으로 되돌린 경우 이름을 밝힌다.
	if len(overwritten) > 0 {
		fmt.Printf("  ⚠ 기존 내용을 내장 버전으로 되돌린 파일 %d개:\n", len(overwritten))
		for _, name := range overwritten {
			fmt.Printf("    ~ .pylon/%s\n", name)
		}
	}
}

func syncSelectedProviderResourcesIfWorkspace(in io.Reader, autoYes bool) {
	root, err := resolveRoot()
	if err != nil {
		return
	}
	cfg, err := config.LoadConfig(layout.ConfigPath(root))
	if err != nil {
		fmt.Printf("⚠ provider 리소스 동기화 건너뜀: %v\n", err)
		return
	}
	catalog, err := newProviderCatalog(cfg)
	if err != nil {
		fmt.Printf("⚠ provider 리소스 동기화 건너뜀: %v\n", err)
		return
	}
	entry, selection, err := catalog.selectInteractive(context.Background(), cfg)
	if err != nil {
		fmt.Printf("⚠ provider 리소스 동기화 건너뜀: %v\n", err)
		return
	}
	if entry.syncDoctor == nil {
		fmt.Printf("✓ provider %s 전용 리소스 동기화 불필요\n", selection.Adapter.Name())
		return
	}
	entry.syncDoctor(in, autoYes)
}

func syncClaudeProviderResourcesIfWorkspace(in io.Reader, autoYes bool) {
	root, err := resolveRoot()
	if err != nil {
		return
	}
	pylonDir := layout.PylonDir(root)

	bootstrapped, backedUp, err := reconcileRootAgentFiles(root)
	if err != nil {
		fmt.Printf("⚠ 루트 에이전트 파일 갱신 실패: %v\n", err)
	} else {
		for _, name := range backedUp {
			fmt.Printf("ℹ 기존 %s를 %s%s로 백업했습니다.\n", name, name, rootFileBackupSuffix)
		}
		if bootstrapped {
			fmt.Println("✓ AGENTS.md를 부트스트랩했습니다 — 다음 실행 시 세션이 이 워크스페이스에 맞게 재작성합니다.")
		}
	}

	cfg, err := config.LoadConfig(filepath.Join(pylonDir, "config.yml"))
	if err != nil {
		if linkErr := syncClaudeAgentLinks(root, pylonDir); linkErr != nil {
			fmt.Printf("⚠ .claude/agents/ 심링크 갱신 실패: %v\n", linkErr)
		}
	} else if genErr := generateClaudeAgentsWithSkills(root, cfg); genErr != nil {
		fmt.Printf("⚠ .claude/agents/ 생성 실패: %v\n", genErr)
	}

	syncClaudeCommandsIfWorkspace(in, autoYes)
}

// syncPylonResources refreshes the pylon-owned resources under .pylon/ from the
// embedded defaults and returns how many files were written plus the labelled
// names of files whose existing content was overwritten.
//
// Ownership contract: a file whose name ships with the binary belongs to pylon and
// is kept identical to the embedded version — editing it in place is not a supported
// customization and the edit is reverted. Every other file in these directories
// (agents from `pylon add-agent`, skills from `pylon add-skill`, any user-authored
// file) belongs to the user and is never written or removed here.
//
// Shared by `pylon doctor` and the launch path so the two can never drift.
func syncPylonResources(pylonDir string) (int, []string) {
	var totalWritten int
	var overwritten []string

	sync := func(fs embed.FS, embedDir, targetDir, suffix, label string) {
		written, refreshed := syncEmbeddedDir(fs, embedDir, targetDir, suffix)
		totalWritten += written
		for _, name := range refreshed {
			overwritten = append(overwritten, label+"/"+name)
		}
	}

	sync(embeddedAgents, "agents", filepath.Join(pylonDir, "agents"), ".md", "agents")
	sync(embeddedSkills, "skills", filepath.Join(pylonDir, "skills"), ".md", "skills")
	sync(embeddedCommands, "commands", filepath.Join(pylonDir, "commands"), ".md", "commands")
	sync(embeddedScripts, "scripts/bash", filepath.Join(pylonDir, "scripts", "bash"), ".sh", "scripts/bash")
	sync(embeddedReference, "reference", filepath.Join(pylonDir, "reference"), ".md", "reference")

	sort.Strings(overwritten)
	return totalWritten, overwritten
}

// syncEmbeddedDir copies files from an embed.FS subdirectory to a target directory.
// .pylon/ resources are treated as pylon-managed: a file is written when it is
// missing or when its on-disk content differs from the embedded version, so that
// version upgrades refresh stale files. Unchanged files are left untouched.
// Returns the number of files written (newly installed or refreshed) and the names
// of files whose existing content was overwritten — those may have been user edits,
// so the caller reports them by name rather than as a bare count.
func syncEmbeddedDir(fs embed.FS, embedDir, targetDir, suffix string) (int, []string) {
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		fmt.Printf("⚠ %s 디렉토리 생성 실패: %v\n", targetDir, err)
		return 0, nil
	}

	entries, err := fs.ReadDir(embedDir)
	if err != nil {
		fmt.Printf("⚠ %s 읽기 실패: %v\n", embedDir, err)
		return 0, nil
	}

	written := 0
	var refreshed []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), suffix) {
			continue // 비재귀: 서브디렉토리 스킵 (현재 리소스 구조에서는 불필요)
		}
		content, err := fs.ReadFile(embedDir + "/" + entry.Name())
		if err != nil {
			fmt.Printf("⚠ %s 읽기 실패: %v\n", entry.Name(), err)
			continue
		}
		destPath := filepath.Join(targetDir, entry.Name())
		existed := false
		if existing, err := os.ReadFile(destPath); err == nil {
			if bytes.Equal(existing, content) {
				continue // 디스크 내용이 내장 버전과 동일 — 갱신 불필요
			}
			existed = true
		}
		perm := os.FileMode(0644)
		if suffix == ".sh" {
			perm = 0755
		}
		if err := os.WriteFile(destPath, content, perm); err != nil {
			fmt.Printf("⚠ %s 쓰기 실패: %v\n", entry.Name(), err)
			continue
		}
		written++
		if existed {
			refreshed = append(refreshed, entry.Name())
		}
	}
	return written, refreshed
}

// commandDiff describes how the on-disk .claude/commands/ differs from the
// desired command set.
type commandDiff struct {
	added   []string // in desired, missing on disk
	changed []string // present but content differs
	removed []string // legacy files present on disk that will be deleted
}

func (d commandDiff) empty() bool {
	return len(d.added) == 0 && len(d.changed) == 0 && len(d.removed) == 0
}

// diffClaudeCommands compares the desired command set against the on-disk
// .claude/commands/. Removals are limited to the fixed legacy list, mirroring
// applyClaudeCommands — user-added commands are never flagged for deletion.
func diffClaudeCommands(commandsDir string, desired map[string]string) commandDiff {
	var d commandDiff
	for rel, content := range desired {
		existing, err := os.ReadFile(filepath.Join(commandsDir, rel))
		if err != nil {
			d.added = append(d.added, rel)
		} else if !bytes.Equal(existing, []byte(content)) {
			d.changed = append(d.changed, rel)
		}
	}
	for _, name := range legacyCommandFiles {
		if _, err := os.Stat(filepath.Join(commandsDir, name+".md")); err == nil {
			d.removed = append(d.removed, name+".md")
		}
	}
	removed := make(map[string]bool)
	for _, rel := range d.removed {
		removed[rel] = true
	}
	for rel := range readManagedClaudeCommandSet(commandsDir) {
		if _, ok := desired[rel]; ok {
			continue
		}
		if removed[rel] {
			continue
		}
		if _, err := os.Stat(filepath.Join(commandsDir, rel)); err == nil {
			d.removed = append(d.removed, rel)
			removed[rel] = true
		}
	}
	sort.Strings(d.added)
	sort.Strings(d.changed)
	sort.Strings(d.removed)
	return d
}

// syncClaudeCommandsIfWorkspace reconciles .claude/commands/ with the desired
// command set when inside a pylon workspace. It prints a diff and only applies
// changes after the user consents (default: No). When autoYes is true the prompt
// is skipped and changes are applied automatically.
func syncClaudeCommandsIfWorkspace(in io.Reader, autoYes bool) {
	root, err := resolveRoot()
	if err != nil {
		return // not in a workspace, skip
	}

	commandsDir := layout.ClaudeCommandsDir(root)
	desired := buildDesiredClaudeCommands(root)
	diff := diffClaudeCommands(commandsDir, desired)

	fmt.Println()
	if diff.empty() {
		fmt.Println("✓ Claude Code 커맨드 최신 상태")
		return
	}

	fmt.Println("🔄 Claude Code 커맨드 변경 사항:")
	for _, f := range diff.added {
		fmt.Printf("  + %s (추가)\n", f)
	}
	for _, f := range diff.changed {
		fmt.Printf("  ~ %s (변경)\n", f)
	}
	for _, f := range diff.removed {
		fmt.Printf("  - %s (삭제)\n", f)
	}

	if !autoYes {
		fmt.Print("적용하시겠습니까? [y/N]: ")
		answer, _ := bufio.NewReader(in).ReadString('\n')
		answer = strings.ToLower(strings.TrimSpace(answer))
		if answer != "y" && answer != "yes" {
			fmt.Println("커맨드 동기화 건너뜀")
			return
		}
	}

	if err := applyClaudeCommands(commandsDir, desired); err != nil {
		fmt.Printf("⚠ 커맨드 동기화 실패: %v\n", err)
		return
	}
	total := len(diff.added) + len(diff.changed) + len(diff.removed)
	fmt.Printf("✓ 커맨드 %d개 동기화 완료\n", total)
}
