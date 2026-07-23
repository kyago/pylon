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
	"github.com/kyago/pylon/internal/history"
	"github.com/kyago/pylon/internal/layout"
	"github.com/kyago/pylon/internal/store"
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
	if err := prepareHistory(root, cfg.History); err != nil {
		return err
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

func prepareHistory(root string, cfg config.HistoryConfig) error {
	if _, err := history.VerifyFossil(); err != nil {
		return fmt.Errorf("fossil 확인 실패 — 'pylon doctor'로 설치 상태를 확인하세요: %w", err)
	}
	if err := history.NewManager(root, cfg, nil, nil).Initialize(); err != nil {
		return fmt.Errorf("fossil 작업 이력 저장소 초기화 실패: %w", err)
	}
	return nil
}

// openWorkspaceStore is a shared helper that finds the workspace root, loads config,
// and opens the SQLite store. Caller must close the returned Store.
func openWorkspaceStore() (string, *config.Config, *store.Store, error) {
	root, err := resolveRoot()
	if err != nil {
		return "", nil, nil, err
	}

	cfg, err := config.LoadConfig(layout.ConfigPath(root))
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to load config: %w", err)
	}

	dbPath := layout.DBPath(root)
	s, err := store.NewStore(dbPath)
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to open store: %w", err)
	}

	if err := s.Migrate(); err != nil {
		s.Close()
		return "", nil, nil, fmt.Errorf("failed to migrate: %w", err)
	}

	return root, cfg, s, nil
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

	// Generate CLAUDE.md at workspace root
	claudeMD := buildRootCLAUDEMD(cfg, projects, root)
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte(claudeMD), 0644); err != nil {
		return fmt.Errorf("CLAUDE.md 생성 실패: %w", err)
	}

	// Reconcile .claude/commands/ with the desired command set. The desired-state
	// computation is shared with `pylon doctor` so the two never drift.
	desired := buildDesiredClaudeCommands(root)
	if err := applyClaudeCommands(commandsDir, desired); err != nil {
		return err
	}

	// Bootstrap .pylon/commands/ from embedded defaults for future customization
	if err := bootstrapPylonCommands(layout.CommandsDir(root)); err != nil {
		return err
	}

	// Pipeline scripts → .pylon/scripts/bash/
	// Bootstrap embedded scripts if workspace doesn't have them yet (existing workspaces)
	pylonScriptsDir := layout.ScriptsDir(root)
	if err := os.MkdirAll(pylonScriptsDir, 0755); err != nil {
		return fmt.Errorf("scripts/bash/ 디렉토리 생성 실패: %w", err)
	}
	scriptEntries, err := embeddedScripts.ReadDir("scripts/bash")
	if err == nil {
		for _, entry := range scriptEntries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sh") {
				continue
			}
			destPath := filepath.Join(pylonScriptsDir, entry.Name())
			// Only write if file doesn't exist (preserve user customizations)
			if _, err := os.Stat(destPath); os.IsNotExist(err) {
				content, err := embeddedScripts.ReadFile("scripts/bash/" + entry.Name())
				if err != nil {
					fmt.Fprintf(os.Stderr, "경고: 내장 스크립트 읽기 실패 (%s): %v\n", entry.Name(), err)
					continue
				}
				if err := os.WriteFile(destPath, content, 0755); err != nil {
					fmt.Fprintf(os.Stderr, "경고: 스크립트 배포 실패 (%s): %v\n", entry.Name(), err)
				}
			}
		}
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
	for _, entry := range []string{".claude/", "CLAUDE.md", ".pylon/logs/"} {
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
