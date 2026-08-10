package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/charmbracelet/huh"
	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/layout"
	"github.com/kyago/pylon/internal/provider"
)

var buildLaunchProviderCatalog = newProviderCatalog
var replaceLaunchProcess = syscall.Exec

// runLaunch is the main entry point when `pylon` is invoked without subcommands.
// It selects an interactive provider, materializes provider-owned resources from
// .pylon/, and replaces the current process with the provider executable.
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

	// Step 4: Select an interactive provider
	catalog, err := buildLaunchProviderCatalog(cfg)
	if err != nil {
		return fmt.Errorf("provider catalog 구성 실패: %w", err)
	}
	entry, selection, err := catalog.selectInteractive(context.Background(), cfg)
	if err != nil {
		return fmt.Errorf("interactive provider 선택 실패: %w", err)
	}

	// Step 5: Materialize provider-owned workspace resources
	if entry.prepareWorkspace != nil {
		if err := entry.prepareWorkspace(root, cfg, projects); err != nil {
			return fmt.Errorf("provider %s workspace 준비 실패: %w", selection.Adapter.Name(), err)
		}
	}

	// Ensure generated provider resources are ignored where applicable.
	ensureGitignore(root)

	// Step 6: Select provider permission mode
	permissionMode := cfg.Runtime.PermissionMode
	if entry.selectPermission != nil {
		permissionMode, err = entry.selectPermission(permissionMode)
		if err != nil {
			return err
		}
	}

	// Step 7: Build and launch the provider process.
	interactiveAdapter, ok := selection.Adapter.(provider.InteractiveAdapter)
	if !ok {
		return fmt.Errorf("provider %s does not support interactive launch", selection.Adapter.Name())
	}
	process, err := interactiveAdapter.PrepareInteractive(context.Background(), provider.InteractiveSpec{
		MaxTurns:            cfg.Runtime.MaxTurns,
		PermissionMode:      permissionMode,
		Environment:         os.Environ(),
		EnvironmentOverride: cfg.Runtime.Env,
	})
	if err != nil {
		return fmt.Errorf("provider %s 실행 준비 실패: %w", selection.Adapter.Name(), err)
	}

	fmt.Printf("%s를 시작합니다...\n", process.DisplayName)
	return replaceLaunchProcess(process.Executable, process.Args, process.Environment)
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
