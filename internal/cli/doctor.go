package cli

import (
	"fmt"
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

func verifyClaude() (string, error) {
	out, err := exec.Command("claude", "--version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("claude not found: %w", err)
	}
	ver := strings.TrimSpace(string(out))
	return ver, nil
}
