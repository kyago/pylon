package cli

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kyago/pylon/internal/config"
	"github.com/spf13/cobra"
)

// Check represents a single dependency check.
// Spec Reference: Section 7 "pylon doctor"
type Check struct {
	Name       string
	Required   bool
	Verify     func() (version string, err error)
	InstallURL string
}

var checks = []Check{
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
	{
		Name:       "claude",
		Required:   true,
		Verify:     verifyClaude,
		InstallURL: "https://docs.anthropic.com/en/docs/claude-code",
	},
}

func newDoctorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check required tool installations and versions",
		Long: `Verify that all required tools (git, gh, claude) are installed and configured.

Use --fix-excludes to automatically add .pylon/ to project .git/info/exclude
for any projects that are missing the local-scope ignore entry.`,
		RunE: runDoctor,
	}

	cmd.Flags().Bool("fix-excludes", false, "auto-fix missing .pylon/ exclude entries in project repos")

	return cmd
}

// runChecks executes all doctor checks and returns results.
func runChecks() (allPassed bool, failures []Check) {
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

	allPassed, failures := runChecks()

	// Sync config defaults if in a workspace
	fmt.Println()
	syncConfigIfWorkspace()

	// Sync embedded resources (agents, skills, commands, scripts) if in a workspace
	syncResourcesIfWorkspace()

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

	fmt.Println()
	if allPassed {
		fmt.Println("All checks passed.")
		return nil
	}

	fmt.Println("Some checks failed. Install missing tools:")
	for _, f := range failures {
		fmt.Printf("  %s: %s\n", f.Name, f.InstallURL)
	}
	return fmt.Errorf("doctor checks failed")
}

// checkRepoExcludes verifies that all projects have .pylon/
// in their .git/info/exclude for local-scope ignore.
// When fix is true, missing entries are automatically added.
// Returns true if all projects are OK, false if any are missing.
func checkRepoExcludes(fix bool) bool {
	startDir := flagWorkspace
	if startDir == "" {
		startDir = "."
	}
	root, err := config.FindWorkspaceRoot(startDir)
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

	allPassed, failures := runChecks()

	fmt.Println()
	if !allPassed {
		fmt.Println("Some checks failed. Install missing tools:")
		for _, f := range failures {
			fmt.Printf("  %s: %s\n", f.Name, f.InstallURL)
		}
	}

	return allPassed, nil
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
	startDir := flagWorkspace
	if startDir == "" {
		startDir = "."
	}
	root, err := config.FindWorkspaceRoot(startDir)
	if err != nil {
		return // not in a workspace, skip
	}

	cfgPath := filepath.Join(root, ".pylon", "config.yml")
	_, added, err := config.SyncConfigDefaults(cfgPath)
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
}

// syncResourcesIfWorkspace syncs embedded agents, skills, commands, and scripts
// to the workspace if running inside a pylon workspace.
// .pylon/ resources are pylon-managed: missing files are installed and files
// whose content differs from the embedded version are refreshed, so upgrades
// pick up new resource content.
func syncResourcesIfWorkspace() {
	startDir := flagWorkspace
	if startDir == "" {
		startDir = "."
	}
	root, err := config.FindWorkspaceRoot(startDir)
	if err != nil {
		return // not in a workspace, skip
	}

	pylonDir := filepath.Join(root, ".pylon")
	var totalWritten int

	// Sync agents
	totalWritten += syncEmbeddedDir(embeddedAgents, "agents", filepath.Join(pylonDir, "agents"), ".md")

	// Sync skills
	totalWritten += syncEmbeddedDir(embeddedSkills, "skills", filepath.Join(pylonDir, "skills"), ".md")

	// Sync commands
	totalWritten += syncEmbeddedDir(embeddedCommands, "commands", filepath.Join(pylonDir, "commands"), ".md")

	// Sync scripts
	totalWritten += syncEmbeddedDir(embeddedScripts, "scripts/bash", filepath.Join(pylonDir, "scripts", "bash"), ".sh")

	// Update .claude/agents/ with skill injection (consistent with pylon launch)
	cfg, err := config.LoadConfig(filepath.Join(pylonDir, "config.yml"))
	if err != nil {
		// Fall back to plain symlinks if config can't be loaded
		if linkErr := syncClaudeAgentLinks(root, pylonDir); linkErr != nil {
			fmt.Printf("⚠ .claude/agents/ 심링크 갱신 실패: %v\n", linkErr)
		}
	} else {
		if genErr := generateClaudeAgentsWithSkills(root, cfg); genErr != nil {
			fmt.Printf("⚠ .claude/agents/ 생성 실패: %v\n", genErr)
		}
	}

	if totalWritten > 0 {
		fmt.Printf("✓ 내장 리소스 %d개 설치/갱신\n", totalWritten)
	} else {
		fmt.Println("✓ 내장 리소스 최신 상태")
	}
}

// syncEmbeddedDir copies files from an embed.FS subdirectory to a target directory.
// .pylon/ resources are treated as pylon-managed: a file is written when it is
// missing or when its on-disk content differs from the embedded version, so that
// version upgrades refresh stale files. Unchanged files are left untouched.
// Returns the number of files written (newly installed or refreshed).
func syncEmbeddedDir(fs embed.FS, embedDir, targetDir, suffix string) int {
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		fmt.Printf("⚠ %s 디렉토리 생성 실패: %v\n", targetDir, err)
		return 0
	}

	entries, err := fs.ReadDir(embedDir)
	if err != nil {
		fmt.Printf("⚠ %s 읽기 실패: %v\n", embedDir, err)
		return 0
	}

	written := 0
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
		if existing, err := os.ReadFile(destPath); err == nil && bytes.Equal(existing, content) {
			continue // 디스크 내용이 내장 버전과 동일 — 갱신 불필요
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
	}
	return written
}

func verifyClaude() (string, error) {
	out, err := exec.Command("claude", "--version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("claude not found: %w", err)
	}
	ver := strings.TrimSpace(string(out))
	return ver, nil
}
