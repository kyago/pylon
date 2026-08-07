package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/charmbracelet/huh"
	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/layout"
)

// runLaunch is the main entry point when `pylon` is invoked without subcommands.
// It generates .claude/ artifacts from .pylon/ (source of truth) and launches
// Claude Code TUI directly via syscall.Exec.
func runLaunch() error {
	// Step 1: Find workspace
	root, err := resolveRoot()
	if err != nil {
		return err
	}

	// Step 2: Load config
	cfg, err := config.LoadConfig(layout.ConfigPath(root))
	if err != nil {
		return fmt.Errorf("설정 로드 실패: %w", err)
	}

	// Step 3: Discover projects
	projects, err := config.DiscoverProjects(root)
	if err != nil {
		// Non-fatal: workspace may not have projects yet
		projects = nil
	}

	// Step 4: Generate .claude/ directory structure
	if err := generateClaudeDir(root, cfg, projects); err != nil {
		return fmt.Errorf(".claude/ 생성 실패: %w", err)
	}

	// Ensure .claude/ and CLAUDE.md are in .gitignore
	ensureGitignore(root)

	// Step 5: Select permission mode
	permMode, err := selectPermissionMode(cfg.Runtime.PermissionMode)
	if err != nil {
		return err
	}

	// Step 6: Launch Claude Code (replace process)
	claudePath, err := exec.LookPath("claude")
	if err != nil {
		return fmt.Errorf("claude 실행 파일을 찾을 수 없습니다: %w", err)
	}

	args := append([]string{"claude"}, buildClaudeArgs(cfg, permMode)...)

	// Build env with config overrides (deduplicated — config wins over existing)
	envMap := make(map[string]string)
	for _, e := range os.Environ() {
		if k, v, ok := strings.Cut(e, "="); ok {
			envMap[k] = v
		}
	}
	for k, v := range cfg.Runtime.Env {
		envMap[k] = v
	}
	env := make([]string, 0, len(envMap))
	for k, v := range envMap {
		env = append(env, k+"="+v)
	}

	fmt.Println("Claude Code를 시작합니다...")
	return syscall.Exec(claudePath, args, env)
}

// openWorkspace finds the workspace root and loads config.
func openWorkspace() (string, *config.Config, error) {
	root, err := resolveRoot()
	if err != nil {
		return "", nil, err
	}
	cfg, err := config.LoadConfig(layout.ConfigPath(root))
	if err != nil {
		return "", nil, fmt.Errorf("failed to load config: %w", err)
	}
	return root, cfg, nil
}

// selectPermissionMode presents an interactive selector for Claude Code permission mode.
func selectPermissionMode(defaultMode string) (string, error) {
	if defaultMode == "" {
		defaultMode = "default"
	}

	modes := []huh.Option[string]{
		huh.NewOption("default — 매번 권한 확인", "default"),
		huh.NewOption("acceptEdits — 파일 편집 자동 허용", "acceptEdits"),
		huh.NewOption("bypassPermissions — 모든 권한 자동 허용", "bypassPermissions"),
	}

	// Pre-select the default from config
	for i, m := range modes {
		if m.Value == defaultMode {
			modes[i] = modes[i].Selected(true)
			break
		}
	}

	var selected string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Permission Mode 선택").
				Description("Claude Code 실행 권한을 설정합니다").
				Options(modes...).
				Value(&selected),
		),
	)

	if err := form.Run(); err != nil {
		return "", fmt.Errorf("선택 취소됨: %w", err)
	}

	return selected, nil
}

// buildClaudeArgs constructs the claude CLI arguments as a string slice.
func buildClaudeArgs(cfg *config.Config, permMode string) []string {
	var args []string

	if cfg.Runtime.MaxTurns > 0 {
		args = append(args, "--max-turns", fmt.Sprintf("%d", cfg.Runtime.MaxTurns))
	}
	args = append(args, "--permission-mode", permMode)

	return args
}

// generateClaudeDir creates/updates the .claude/ directory from .pylon/ source of truth.
func generateClaudeDir(root string, cfg *config.Config, projects []config.ProjectInfo) error {
	claudeDir := layout.ClaudeDir(root)
	commandsDir := filepath.Join(claudeDir, "commands")

	// Ensure directories exist
	if err := os.MkdirAll(commandsDir, 0755); err != nil {
		return err
	}

	// Root agent files: CLAUDE.md is a deterministic @AGENTS.md import marker; AGENTS.md
	// is (re)bootstrapped only when missing/stale so a session-authored guide survives
	// launch. The launched claude session authors AGENTS.md on its first turn.
	bootstrapped, backedUp, err := ensureRootAgentFiles(root, projects)
	if err != nil {
		return err
	}
	for _, name := range backedUp {
		fmt.Fprintf(os.Stderr, "ℹ 기존 %s를 %s%s로 백업했습니다.\n", name, name, rootFileBackupSuffix)
	}
	if bootstrapped {
		fmt.Fprintln(os.Stderr, "ℹ AGENTS.md를 부트스트랩했습니다 — 세션이 첫 턴에 이 워크스페이스에 맞게 재작성합니다.")
	}

	// Refresh the pylon-owned resources under .pylon/ (agents, skills, commands,
	// scripts) from the embedded defaults. Shared with `pylon doctor` so launch and
	// maintenance can never drift, and runs before the .claude/commands/
	// reconciliation below so a command added in a new release reaches Claude Code on
	// this launch rather than the next one. User-authored files are left untouched —
	// see syncPylonResources for the ownership contract.
	if _, overwritten := syncPylonResources(layout.PylonDir(root)); len(overwritten) > 0 {
		fmt.Fprintf(os.Stderr, "⚠ 내장 버전으로 되돌린 pylon 소유 파일 %d개: %s\n",
			len(overwritten), strings.Join(overwritten, ", "))
	}

	// Reconcile .claude/commands/ with the desired command set. The desired-state
	// computation is shared with `pylon doctor` so the two never drift.
	desired := buildDesiredClaudeCommands(root)
	if err := applyClaudeCommands(commandsDir, desired); err != nil {
		return err
	}

	// Generate .claude/agents/ with skill injection
	if err := generateClaudeAgentsWithSkills(root, cfg); err != nil {
		return fmt.Errorf(".claude/agents/ 생성 실패: %w", err)
	}

	// Generate hooks in .claude/settings.json for Claude Code session lifecycle
	if err := generateSettingsHooks(claudeDir); err != nil {
		return fmt.Errorf("settings.json hooks 생성 실패: %w", err)
	}

	return nil
}

// addToGitignore appends pylon-managed entries to .gitignore if not already present.
func addClaudeDirToGitignore(root string) error {
	gitignorePath := filepath.Join(root, ".gitignore")

	existing, _ := os.ReadFile(gitignorePath)
	content := string(existing)

	// Collect missing entries
	var missing []string
	for _, entry := range []string{".claude/", "CLAUDE.md", "AGENTS.md", ".pylon/logs/"} {
		if !strings.Contains(content, entry) {
			missing = append(missing, entry)
		}
	}
	if len(missing) == 0 {
		return nil
	}

	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	var b strings.Builder
	b.WriteString("\n# Pylon-generated (dynamically generated)\n")
	for _, entry := range missing {
		b.WriteString(entry + "\n")
	}
	_, err = f.WriteString(b.String())
	return err
}

// ensureGitignore is called on first launch to add .claude/ to .gitignore.
func ensureGitignore(root string) {
	_ = addClaudeDirToGitignore(root)
}
