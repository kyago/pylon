package cli

import (
	"fmt"
	"os"
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
		t.Errorf("AGENTS.md bootstrap missing: %v", err)
	}
}

func TestGenerateClaudeDirDoesNotClobberAuthoredAgentsMD(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(layout.PylonDir(root), 0755); err != nil {
		t.Fatal(err)
	}
	authored := fmt.Sprintf("<!-- pylon-usage-version: %d -->\n# 저작된 가이드 (보존되어야 함)", pylonUsageVersion)
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte(authored), 0644); err != nil {
		t.Fatal(err)
	}
	if err := generateClaudeDir(root, &config.Config{}, nil); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(layout.RootAgentsPath(root))
	if string(got) != authored {
		t.Errorf("authored AGENTS.md was overwritten: %q", got)
	}
}
