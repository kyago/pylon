package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kyago/pylon/internal/config"
	"github.com/spf13/cobra"
)

func newUninstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Completely remove pylon from workspace",
		Long: `Completely remove pylon from the current workspace.

This removes:
  1. Runtime artifacts (.claude/, CLAUDE.md)
  2. Project-level .pylon/ directories in submodules
  3. Workspace .pylon/ directory (config, domain, agents, database)
  4. Pylon entries from .gitignore

Git submodules and project code are preserved by default.

Use --remove-projects to also remove git submodule registrations.
Use --remove-binary to also delete the pylon binary from $GOPATH/bin.`,
		RunE: runUninstall,
	}

	cmd.Flags().BoolP("force", "f", false, "skip confirmation prompt")
	cmd.Flags().Bool("dry-run", false, "show what would be removed without removing")
	cmd.Flags().Bool("remove-projects", false, "also remove git submodule registrations")
	cmd.Flags().Bool("remove-binary", false, "also remove the pylon binary from $GOPATH/bin")

	return cmd
}

// uninstallPlan holds the list of actions to perform during uninstall.
type uninstallPlan struct {
	runtimeFiles   []string // .claude/, CLAUDE.md
	projectPylons  []string // {project}/.pylon/ directories
	submodules     []string // git submodule names (only if --remove-projects)
	workspacePylon string   // .pylon/ directory
	gitignorePath  string   // .gitignore to clean
	binaryPath     string   // pylon binary path (only if --remove-binary)
}

func runUninstall(cmd *cobra.Command, args []string) error {
	force, _ := cmd.Flags().GetBool("force")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	removeProjects, _ := cmd.Flags().GetBool("remove-projects")
	removeBinary, _ := cmd.Flags().GetBool("remove-binary")

	// Find workspace
	startDir := flagWorkspace
	if startDir == "" {
		startDir = "."
	}
	root, err := config.FindWorkspaceRoot(startDir)
	if err != nil {
		return fmt.Errorf("pylon 워크스페이스가 아닙니다: %w", err)
	}

	// Build uninstall plan
	plan, err := buildUninstallPlan(root, removeProjects, removeBinary)
	if err != nil {
		return fmt.Errorf("언인스톨 계획 생성 실패: %w", err)
	}

	// Show plan
	printUninstallPlan(plan)

	if dryRun {
		fmt.Println("\n(dry-run 모드: 실제 삭제하지 않습니다)")
		return nil
	}

	// Confirm
	if !force {
		fmt.Print("\n정말 진행하시겠습니까? (y/N): ")
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			fmt.Println("언인스톨이 취소되었습니다.")
			return nil
		}
	}

	// Execute uninstall
	return executeUninstall(root, plan, removeProjects)
}

func buildUninstallPlan(root string, removeProjects, removeBinary bool) (*uninstallPlan, error) {
	plan := &uninstallPlan{}

	// 1. Runtime artifacts
	claudeDir := filepath.Join(root, ".claude")
	if dirExists(claudeDir) {
		plan.runtimeFiles = append(plan.runtimeFiles, claudeDir)
	}
	claudeMD := filepath.Join(root, "CLAUDE.md")
	if fileExists(claudeMD) {
		plan.runtimeFiles = append(plan.runtimeFiles, claudeMD)
	}

	// 2. Discover projects and their .pylon/ directories
	projects, _ := config.DiscoverProjects(root)
	for _, p := range projects {
		projectPylon := filepath.Join(p.Path, ".pylon")
		if dirExists(projectPylon) {
			plan.projectPylons = append(plan.projectPylons, projectPylon)
		}
		if removeProjects {
			plan.submodules = append(plan.submodules, p.Name)
		}
	}

	// 3. Workspace .pylon/
	pylonDir := filepath.Join(root, ".pylon")
	if dirExists(pylonDir) {
		plan.workspacePylon = pylonDir
	}

	// 4. .gitignore
	gitignorePath := filepath.Join(root, ".gitignore")
	if fileExists(gitignorePath) {
		plan.gitignorePath = gitignorePath
	}

	// 5. Binary
	if removeBinary {
		binaryPath, err := findPylonBinary()
		if err == nil && binaryPath != "" {
			plan.binaryPath = binaryPath
		}
	}

	return plan, nil
}

func printUninstallPlan(plan *uninstallPlan) {
	fmt.Println("다음 항목이 제거됩니다:")
	fmt.Println()

	if len(plan.runtimeFiles) > 0 {
		fmt.Println("  [런타임 아티팩트]")
		for _, f := range plan.runtimeFiles {
			fmt.Printf("    - %s\n", f)
		}
	}

	if len(plan.projectPylons) > 0 {
		fmt.Println("  [프로젝트 .pylon/ 디렉토리]")
		for _, p := range plan.projectPylons {
			fmt.Printf("    - %s\n", p)
		}
	}

	if len(plan.submodules) > 0 {
		fmt.Println("  [Git 서브모듈 등록 해제]")
		for _, s := range plan.submodules {
			fmt.Printf("    - %s\n", s)
		}
	}

	if plan.workspacePylon != "" {
		fmt.Println("  [워크스페이스]")
		fmt.Printf("    - %s\n", plan.workspacePylon)
	}

	if plan.gitignorePath != "" {
		fmt.Println("  [.gitignore 정리]")
		fmt.Printf("    - %s (pylon 관련 엔트리 제거)\n", plan.gitignorePath)
	}

	if plan.binaryPath != "" {
		fmt.Println("  [바이너리]")
		fmt.Printf("    - %s\n", plan.binaryPath)
	}
}

func executeUninstall(root string, plan *uninstallPlan, removeProjects bool) error {
	var errors []string

	// Step 1: Remove runtime artifacts
	for _, f := range plan.runtimeFiles {
		if err := os.RemoveAll(f); err != nil {
			errors = append(errors, fmt.Sprintf("런타임 아티팩트 제거 실패 (%s): %v", f, err))
		} else {
			fmt.Printf("✓ 제거: %s\n", f)
		}
	}

	// Step 2: Remove project-level .pylon/ directories
	for _, p := range plan.projectPylons {
		if err := os.RemoveAll(p); err != nil {
			errors = append(errors, fmt.Sprintf("프로젝트 .pylon/ 제거 실패 (%s): %v", p, err))
		} else {
			fmt.Printf("✓ 제거: %s\n", p)
		}
	}

	// Step 3: Remove git submodules (if requested)
	if removeProjects {
		for _, name := range plan.submodules {
			if err := removeSubmodule(root, name); err != nil {
				errors = append(errors, fmt.Sprintf("서브모듈 제거 실패 (%s): %v", name, err))
			} else {
				fmt.Printf("✓ 서브모듈 제거: %s\n", name)
			}
		}
	}

	// Step 4: Remove workspace .pylon/
	if plan.workspacePylon != "" {
		if err := os.RemoveAll(plan.workspacePylon); err != nil {
			errors = append(errors, fmt.Sprintf("워크스페이스 .pylon/ 제거 실패: %v", err))
		} else {
			fmt.Printf("✓ 제거: %s\n", plan.workspacePylon)
		}
	}

	// Step 5: Clean .gitignore
	if plan.gitignorePath != "" {
		if err := cleanGitignoreFull(plan.gitignorePath); err != nil {
			errors = append(errors, fmt.Sprintf(".gitignore 정리 실패: %v", err))
		} else {
			fmt.Printf("✓ .gitignore 정리 완료\n")
		}
	}

	// Step 6: Remove binary
	if plan.binaryPath != "" {
		if err := os.Remove(plan.binaryPath); err != nil {
			errors = append(errors, fmt.Sprintf("바이너리 제거 실패 (%s): %v", plan.binaryPath, err))
		} else {
			fmt.Printf("✓ 바이너리 제거: %s\n", plan.binaryPath)
		}
	}

	fmt.Println()
	if len(errors) > 0 {
		fmt.Println("일부 항목 제거에 실패했습니다:")
		for _, e := range errors {
			fmt.Printf("  ⚠ %s\n", e)
		}
		return fmt.Errorf("%d개 항목 제거 실패", len(errors))
	}

	fmt.Println("Pylon이 완전히 제거되었습니다.")
	return nil
}

// removeSubmodule removes a git submodule registration and its cached data.
func removeSubmodule(root, name string) error {
	// git submodule deinit
	deinitCmd := exec.Command("git", "submodule", "deinit", "-f", name)
	deinitCmd.Dir = root
	if output, err := deinitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("deinit 실패: %w\n%s", err, output)
	}

	// git rm
	rmCmd := exec.Command("git", "rm", "-f", name)
	rmCmd.Dir = root
	if output, err := rmCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git rm 실패: %w\n%s", err, output)
	}

	// Remove cached module data
	gitModulesDir := filepath.Join(root, ".git", "modules", name)
	os.RemoveAll(gitModulesDir)

	return nil
}

// cleanGitignoreFull removes all pylon-related entries from .gitignore,
// including both workspace and Claude Code generated entries.
func cleanGitignoreFull(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	lines := strings.Split(string(data), "\n")
	var cleaned []string
	pylonSection := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect pylon section markers (both workspace and generated)
		if trimmed == "# pylon" || trimmed == "# Pylon" ||
			trimmed == "# Pylon runtime (agent communication, state)" ||
			trimmed == "# Pylon-generated Claude Code config (dynamically generated)" {
			pylonSection = true
			continue
		}

		if pylonSection {
			// Skip pylon-specific entries
			if trimmed == "" {
				pylonSection = false
				continue
			}
			if strings.HasPrefix(trimmed, ".pylon/") ||
				strings.HasPrefix(trimmed, ".claude/") ||
				trimmed == "CLAUDE.md" {
				continue
			}
			// Non-pylon entry ends the section
			pylonSection = false
		}

		cleaned = append(cleaned, line)
	}

	// Remove trailing empty lines that may result from section removal
	for len(cleaned) > 0 && strings.TrimSpace(cleaned[len(cleaned)-1]) == "" {
		cleaned = cleaned[:len(cleaned)-1]
	}
	if len(cleaned) > 0 {
		cleaned = append(cleaned, "") // ensure final newline
	}

	return os.WriteFile(path, []byte(strings.Join(cleaned, "\n")), 0644)
}

// findPylonBinary locates the pylon binary in $GOPATH/bin.
func findPylonBinary() (string, error) {
	// Try GOPATH/bin first
	goPathCmd := exec.Command("go", "env", "GOPATH")
	output, err := goPathCmd.Output()
	if err == nil {
		goPath := strings.TrimSpace(string(output))
		if goPath != "" {
			binaryPath := filepath.Join(goPath, "bin", "pylon")
			if fileExists(binaryPath) {
				return binaryPath, nil
			}
		}
	}

	// Fallback: check if pylon is in PATH
	path, err := exec.LookPath("pylon")
	if err != nil {
		return "", fmt.Errorf("pylon 바이너리를 찾을 수 없습니다")
	}
	return path, nil
}
