package cli

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
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
		Name:       "tmux",
		Required:   true,
		Verify:     verifyTmux,
		InstallURL: "https://github.com/tmux/tmux/wiki/Installing",
	},
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
		Long:  "Verify that all required tools (tmux, git, gh, claude) are installed and configured.",
		RunE:  runDoctor,
	}
}

func runDoctor(cmd *cobra.Command, args []string) error {
	fmt.Println("Pylon Doctor")
	fmt.Println(strings.Repeat("\u2500", 40))

	allPassed := true
	var failures []Check

	for _, check := range checks {
		ver, err := check.Verify()
		if err != nil {
			allPassed = false
			failures = append(failures, check)
			reqLabel := ""
			if check.Required {
				reqLabel = " [required]"
			}
			fmt.Printf("\u2717 %-10s %-10s%s\n", check.Name, "missing", reqLabel)
		} else {
			reqLabel := ""
			if check.Required {
				reqLabel = " [required]"
			}
			fmt.Printf("\u2713 %-10s %-10s%s\n", check.Name, ver, reqLabel)
		}
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

// RunDoctorChecks runs doctor checks and returns whether all passed.
// Used internally by pylon init.
func RunDoctorChecks() (bool, error) {
	allPassed := true
	for _, check := range checks {
		if _, err := check.Verify(); err != nil {
			allPassed = false
		}
	}
	return allPassed, nil
}

func verifyTmux() (string, error) {
	out, err := exec.Command("tmux", "-V").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("tmux not found: %w", err)
	}
	// tmux -V outputs "tmux 3.4" or similar
	ver := strings.TrimSpace(string(out))
	ver = strings.TrimPrefix(ver, "tmux ")
	return ver, nil
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

func verifyClaude() (string, error) {
	out, err := exec.Command("claude", "--version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("claude not found: %w", err)
	}
	ver := strings.TrimSpace(string(out))
	return ver, nil
}
