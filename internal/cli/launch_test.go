package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/layout"
)

func TestGenerateClaudeDirWritesPointerAndBootstrap(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(layout.PylonDir(root), 0755); err != nil {
		t.Fatal(err)
	}
	if err := generateClaudeDir(root, &config.Config{}, nil); err != nil {
		t.Fatal(err)
	}
	ptr, _ := os.ReadFile(layout.RootClaudePath(root))
	if strings.TrimSpace(string(ptr)) != "@AGENTS.md" {
		t.Errorf("CLAUDE.md should be @AGENTS.md marker, got %q", ptr)
	}
	if _, err := os.Stat(layout.RootAgentsPath(root)); err != nil {
		t.Errorf("AGENTS.md missing: %v", err)
	}
}

func TestGenerateClaudeDirKeepsUserRulesAndLinksBuiltinAgents(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(layout.PylonDir(root), 0755); err != nil {
		t.Fatal(err)
	}
	userRules := "# 우리 팀 규칙 (보존되어야 함)\n"
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte(userRules), 0644); err != nil {
		t.Fatal(err)
	}
	if err := generateClaudeDir(root, &config.Config{}, nil); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(layout.RootAgentsPath(root))
	if string(got) != buildAgentsBlock(nil)+userRules {
		t.Errorf("user rules must survive behind the generated block: %q", got)
	}
	if _, err := os.Stat(filepath.Join(layout.ClaudeAgentsDir(root), "architect.md")); err != nil {
		t.Errorf("built-in agent link missing or dangling: %v", err)
	}
}
