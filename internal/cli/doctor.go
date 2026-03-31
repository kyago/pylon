package cli

import (
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/kyago/pylon/internal/config"
)

// Check represents a single dependency check.
// Spec Reference: Section 7 "pylon doctor"
type Check struct {
	Name     string
	Required bool
	Verify   func() (version string, err error)
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
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check required tool installations and versions",
		Long:  "Verify that all required tools (git, gh, claude) are installed and configured.",
		RunE:  runDoctor,
	}
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
// New files are installed; existing files are skipped to preserve user customizations.
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
	var totalInstalled int

	// Sync agents
	installed := syncEmbeddedDir(embeddedAgents, "agents", filepath.Join(pylonDir, "agents"), ".md")
	totalInstalled += installed

	// Sync skills
	installed = syncEmbeddedDir(embeddedSkills, "skills", filepath.Join(pylonDir, "skills"), ".md")
	totalInstalled += installed

	// Sync commands
	installed = syncEmbeddedDir(embeddedCommands, "commands", filepath.Join(pylonDir, "commands"), ".md")
	totalInstalled += installed

	// Sync scripts
	installed = syncEmbeddedDir(embeddedScripts, "scripts/bash", filepath.Join(pylonDir, "scripts", "bash"), ".sh")
	totalInstalled += installed

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

	if totalInstalled > 0 {
		fmt.Printf("✓ 내장 리소스 %d개 신규 설치\n", totalInstalled)
	} else {
		fmt.Println("✓ 내장 리소스 최신 상태")
	}
}

// syncEmbeddedDir copies files from an embed.FS subdirectory to a target directory.
// Only new files are installed; existing files are skipped.
// Returns the number of newly installed files.
func syncEmbeddedDir(fs embed.FS, embedDir, targetDir, suffix string) int {
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		fmt.Printf("⚠ %s 디렉토리 생성 실패: %v\n", targetDir, err)
		return 0
	}

	entries, err := fs.ReadDir(embedDir)
	if err != nil {
		return 0
	}

	installed := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), suffix) {
			continue
		}
		destPath := filepath.Join(targetDir, entry.Name())
		if _, err := os.Stat(destPath); err == nil {
			continue // already exists, skip
		}
		content, err := fs.ReadFile(embedDir + "/" + entry.Name())
		if err != nil {
			fmt.Printf("⚠ %s 읽기 실패: %v\n", entry.Name(), err)
			continue
		}
		perm := os.FileMode(0644)
		if suffix == ".sh" {
			perm = 0755
		}
		if err := os.WriteFile(destPath, content, perm); err != nil {
			fmt.Printf("⚠ %s 쓰기 실패: %v\n", entry.Name(), err)
			continue
		}
		installed++
	}
	return installed
}

func verifyClaude() (string, error) {
	out, err := exec.Command("claude", "--version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("claude not found: %w", err)
	}
	ver := strings.TrimSpace(string(out))
	return ver, nil
}
