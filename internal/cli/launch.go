package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/layout"
)

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
	// from the embedded set. Runs before the .claude/ generation below
	// so the agent links resolve into an up-to-date builtin dir. User-authored files
	// are left untouched — see syncBuiltinResources for the ownership contract.
	if _, err := syncBuiltinResources(root); err != nil {
		return fmt.Errorf("내장 리소스 동기화 실패: %w", err)
	}

	// Reconcile .claude/commands/ with the desired command set.
	desired := buildDesiredClaudeCommands(root)
	if err := applyClaudeCommands(commandsDir, desired); err != nil {
		return err
	}

	if err := syncClaudeAgentLinks(root); err != nil {
		return fmt.Errorf(".claude/agents/ 생성 실패: %w", err)
	}

	// Earlier releases injected memory from a SessionStart hook; knowledge is now looked up on demand.
	if _, err := removePylonHooks(claudeDir); err != nil {
		fmt.Fprintf(os.Stderr, "경고: .claude/settings.json 훅 제거 실패: %v\n", err)
	}

	return nil
}

// addToGitignore appends pylon-managed entries to .gitignore if not already present.
func addClaudeDirToGitignore(root string) error {
	gitignorePath := filepath.Join(root, ".gitignore")

	existing, _ := os.ReadFile(gitignorePath)
	present := map[string]bool{}
	for _, line := range strings.Split(string(existing), "\n") {
		present[strings.TrimSpace(line)] = true
	}

	// Whole-line match: ".claude/agents/" (old init) must not count as ".claude/".
	var missing []string
	for _, entry := range []string{".claude/", "CLAUDE.md", "AGENTS.md", ".pylon/builtin/"} {
		if !present[entry] {
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
