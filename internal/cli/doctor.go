package cli

import (
	"bufio"
	"bytes"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/layout"
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
		Short: "Check required tools and refresh the workspace files",
		Long: `Verify that required tools are installed and refresh the generated workspace files.

Also restores the pylon-owned .pylon/builtin/ (agents, commands, manual) to
the versions shipped with this binary. Your own resources in .pylon/agents/,
.pylon/commands/ are left alone; a built-in with the same name wins.

Use --fix-excludes to automatically add .pylon/ to project .git/info/exclude
for any projects that are missing the local-scope ignore entry.`,
		RunE: runDoctor,
	}

	cmd.Flags().Bool("fix-excludes", false, "auto-fix missing .pylon/ exclude entries in project repos")
	cmd.Flags().Bool("yes", false, "확인 프롬프트(커맨드 동기화, provider 선택)를 건너뛰고 비인터랙티브로 실행")

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

	// Sync the embedded resources and the files generated from them if in a workspace.
	fmt.Println()
	syncResourcesIfWorkspace()
	autoYes, _ := cmd.Flags().GetBool("yes")
	syncGeneratedFilesIfWorkspace(cmd.InOrStdin(), autoYes)

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

	if root, err := resolveRoot(); err == nil {
		if projects, err := config.DiscoverProjects(root); err == nil {
			hintUnscaffoldedProjects(root, projects)
		}
	}

	fmt.Println()
	if allPassed {
		fmt.Println("All checks passed.")
		return nil
	}

	// 도구 미설치가 아닌 실패(예: exclude 누락)는 위에서 이미 경고로 안내됐다.
	if len(failures) > 0 {
		printInstallHints(failures)
	}
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
		if config.IsWorkspaceWorktree(root, filepath.Join(root, name)) {
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
	return baseChecks
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

// reconcileRootAgentFiles refreshes the CLAUDE.md marker and the pylon block in
// AGENTS.md, so `pylon doctor` recovers a workspace whose root files drifted. Returns
// whether AGENTS.md was written and the names of any root files backed up first.
func reconcileRootAgentFiles(root string) (bool, []string, error) {
	projects, err := config.DiscoverProjects(root)
	if err != nil {
		projects = nil // 탐색 실패 시 프로젝트 목록 없이라도 블록을 최신화한다
	}
	return ensureRootAgentFiles(root, projects)
}

// syncResourcesIfWorkspace refreshes the pylon-owned .pylon/builtin/ resources if
// running inside a pylon workspace.
func syncResourcesIfWorkspace() {
	root, err := resolveRoot()
	if err != nil {
		return // not in a workspace, skip
	}

	reportBuiltinSync(syncBuiltinResources(root))
	ensureGitignore(root) // .pylon/builtin/ 이 launch 전에도 git에 잡히지 않도록
}

// reportBuiltinSync prints the outcome of syncBuiltinResources for doctor and sync-agents.
func reportBuiltinSync(wrote bool, err error) {
	if err != nil {
		fmt.Printf("⚠ 내장 리소스 동기화 실패: %v\n", err)
		return
	}
	if wrote {
		fmt.Println("✓ 내장 리소스 갱신 (.pylon/builtin/)")
	} else {
		fmt.Println("✓ 내장 리소스 최신 상태")
	}
}

func syncGeneratedFilesIfWorkspace(in io.Reader, autoYes bool) {
	root, err := resolveRoot()
	if err != nil {
		return
	}
	updated, backedUp, err := reconcileRootAgentFiles(root)
	if err != nil {
		fmt.Printf("⚠ 루트 에이전트 파일 갱신 실패: %v\n", err)
	} else {
		for _, name := range backedUp {
			fmt.Printf("ℹ 기존 내용을 %s에 백업했습니다.\n", name)
		}
		if updated {
			fmt.Println("✓ AGENTS.md의 pylon 블록을 갱신했습니다")
		}
	}

	if err := syncClaudeAgentLinks(root); err != nil {
		fmt.Printf("⚠ .claude/agents/ 심링크 갱신 실패: %v\n", err)
	}
	pruneCodexSkills(root)

	// .claude/는 위 에이전트 동기화가 만든다.
	if err := generateSettingsHooks(layout.ClaudeDir(root)); err != nil {
		fmt.Printf("⚠ .claude/settings.json 훅 갱신 실패: %v\n", err)
	} else {
		fmt.Println("✓ .claude/settings.json 훅 갱신")
	}

	syncClaudeCommandsIfWorkspace(in, autoYes)
}

// builtinFile is one embedded resource, addressed by its slash path under
// .pylon/builtin/ (e.g. "agents/architect.md").
type builtinFile struct {
	rel     string
	content []byte
}

// embeddedBuiltinFiles lists every embedded resource in a stable order.
func embeddedBuiltinFiles() ([]builtinFile, error) {
	sets := []struct {
		fs  embed.FS
		dir string
	}{
		{embeddedAgents, "agents"},
		{embeddedCommands, "commands"},
		{embeddedReference, "reference"},
	}
	var files []builtinFile
	for _, set := range sets {
		entries, err := set.fs.ReadDir(set.dir)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			rel := set.dir + "/" + entry.Name()
			content, err := set.fs.ReadFile(rel)
			if err != nil {
				return nil, err
			}
			files = append(files, builtinFile{rel: rel, content: content})
		}
	}
	return files, nil
}

// syncBuiltinResources keeps .pylon/builtin/ identical to the embedded resources and
// returns whether anything under it changed.
//
// Ownership contract: .pylon/builtin/ belongs to pylon. Every run compares each file
// with the embeds, rewrites the missing or differing ones and deletes files the embeds
// no longer have — so a dropped built-in disappears with no migration step and a
// damaged builtin heals on the next doctor. Nothing is removed wholesale, so
// concurrent runs converge instead of racing. User files in .pylon/<kind>/ are never
// touched; readers take the built-in first, so a same-named user file is ignored.
// Shared by init, doctor and `pylon mem inject` so they can never drift.
func syncBuiltinResources(root string) (bool, error) {
	files, err := embeddedBuiltinFiles()
	if err != nil {
		return false, fmt.Errorf("내장 리소스 읽기 실패: %w", err)
	}

	builtinDir := layout.BuiltinDir(root)
	changed := false
	want := make(map[string]bool, len(files))
	for _, f := range files {
		want[f.rel] = true
		target := filepath.Join(builtinDir, filepath.FromSlash(f.rel))
		perm := os.FileMode(0o644)
		if strings.HasSuffix(f.rel, ".sh") {
			perm = 0o755
		}
		if current, err := os.ReadFile(target); err != nil || !bytes.Equal(current, f.content) {
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return changed, err
			}
			if err := os.WriteFile(target, f.content, perm); err != nil {
				return changed, fmt.Errorf(".pylon/builtin/%s 쓰기 실패: %w", f.rel, err)
			}
			changed = true
		}
		if info, err := os.Stat(target); err == nil && info.Mode().Perm() != perm {
			if err := os.Chmod(target, perm); err != nil {
				return changed, err
			}
			changed = true
		}
	}
	err = filepath.WalkDir(builtinDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil // 동시 실행이 먼저 지웠다
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(builtinDir, p)
		if err != nil || want[filepath.ToSlash(rel)] {
			return err
		}
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
		changed = true
		return nil
	})
	if err != nil {
		return changed, fmt.Errorf(".pylon/builtin 정리 실패: %w", err)
	}
	return changed, nil
}

// syncBuiltinForLaunch is the launch-path wrapper around syncBuiltinResources.
func syncBuiltinForLaunch(root string) error {
	if _, err := syncBuiltinResources(root); err != nil {
		return fmt.Errorf("내장 리소스 동기화 실패: %w", err)
	}
	return nil
}

// commandDiff describes how the on-disk .claude/commands/ differs from the
// desired command set.
type commandDiff struct {
	added   []string // in desired, missing on disk
	changed []string // present but content differs
	removed []string // previously managed files no longer desired, deleted on apply
}

func (d commandDiff) empty() bool {
	return len(d.added) == 0 && len(d.changed) == 0 && len(d.removed) == 0
}

// diffClaudeCommands compares the desired command set against the on-disk
// .claude/commands/. Removals are limited to previously managed files, mirroring
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
	for rel := range readManagedClaudeCommandSet(commandsDir) {
		if _, ok := desired[rel]; ok {
			continue
		}
		if _, err := os.Stat(filepath.Join(commandsDir, rel)); err == nil {
			d.removed = append(d.removed, rel)
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
