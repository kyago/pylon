package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/layout"
)

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

// generateClaudeDir creates/updates the .claude/ directory from .pylon/ source of truth.
func generateClaudeDir(root string, projects []config.ProjectInfo) error {
	claudeDir := layout.ClaudeDir(root)
	commandsDir := filepath.Join(claudeDir, "commands")

	// Ensure directories exist
	if err := os.MkdirAll(commandsDir, 0755); err != nil {
		return err
	}

	// Root agent files: CLAUDE.md is a deterministic @AGENTS.md import marker; the pylon
	// block in AGENTS.md is regenerated (embedded manual + workspace facts) whenever it
	// differs. Content outside the block is never touched.
	_, backedUp, err := ensureRootAgentFiles(root, projects)
	if err != nil {
		return err
	}
	for _, name := range backedUp {
		fmt.Fprintf(os.Stderr, "ℹ 기존 내용을 %s에 백업했습니다.\n", name)
	}

	// Refresh the pylon-owned .pylon/builtin/ resources (agents, commands, reference)
	// from the embedded set. Shared with `pylon doctor` so launch
	// and maintenance can never drift, and runs before the .claude/ generation below
	// so the agent links resolve into an up-to-date builtin dir. User-authored files
	// are left untouched — see syncBuiltinResources for the ownership contract.
	if err := syncBuiltinForLaunch(root); err != nil {
		return err
	}

	// Reconcile .claude/commands/ with the desired command set. The desired-state
	// computation is shared with `pylon doctor` so the two never drift.
	desired := buildDesiredClaudeCommands(root)
	if err := applyClaudeCommands(commandsDir, desired); err != nil {
		return err
	}

	if err := syncClaudeAgentLinks(root); err != nil {
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
	for _, entry := range []string{".claude/", "CLAUDE.md", "AGENTS.md", ".pylon/builtin/"} {
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

// ensureGitignore adds the generated paths to .gitignore.
func ensureGitignore(root string) {
	_ = addClaudeDirToGitignore(root)
}
