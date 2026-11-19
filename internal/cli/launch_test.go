package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kyago/pylon/internal/layout"
)

func TestGenerateClaudeDirWritesPointerAndBootstrap(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(layout.PylonDir(root), 0755); err != nil {
		t.Fatal(err)
	}
	if err := generateClaudeDir(root, nil); err != nil {
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
	if err := generateClaudeDir(root, nil); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(layout.RootAgentsPath(root))
	if string(got) != buildAgentsBlock(nil)+userRules {
		t.Errorf("user rules must survive behind the generated block: %q", got)
	}
	if _, err := os.Stat(filepath.Join(layout.ClaudeAgentsDir(root), "dev.md")); err != nil {
		t.Errorf("built-in agent link missing or dangling: %v", err)
	}
}

// 구버전 init은 ".claude/agents/"만 ignore했다. 부분 문자열 매칭이면 ".claude/"가
// 이미 있다고 오인해 .claude/commands/가 추적 대상으로 남는다.
func TestAddClaudeDirToGitignore_WholeLineMatch(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".gitignore")
	if err := os.WriteFile(path, []byte("# old\n.claude/agents/\nCLAUDE.md\nAGENTS.md\n.pylon/builtin/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := addClaudeDirToGitignore(root); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	lines := strings.Split(string(data), "\n")
	if !slices.Contains(lines, ".claude/") {
		t.Fatalf(".claude/ 줄이 추가되어야 한다:\n%s", data)
	}
	if strings.Count(string(data), "CLAUDE.md\n") != 1 {
		t.Fatalf("이미 있는 항목은 중복 추가하지 않아야 한다:\n%s", data)
	}
}
