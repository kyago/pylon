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

func TestGenerateClaudeDirKeepsUserRulesAndLinksUserAgents(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(layout.PylonDir(root), 0755); err != nil {
		t.Fatal(err)
	}
	userAgent := filepath.Join(layout.ResourceDir(root, "agents"), "mine.md")
	if err := os.MkdirAll(filepath.Dir(userAgent), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userAgent, []byte("---\nname: mine\ndescription: x\n---\n"), 0644); err != nil {
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
	if string(got) != buildAgentsBlock(nil, false)+userRules {
		t.Errorf("user rules must survive behind the generated block: %q", got)
	}
	if _, err := os.Stat(filepath.Join(layout.ClaudeAgentsDir(root), "mine.md")); err != nil {
		t.Errorf("user agent link missing or dangling: %v", err)
	}
	if entries, _ := os.ReadDir(layout.ClaudeAgentsDir(root)); len(entries) != 1 {
		t.Errorf("pylon ships no agents, so only the user's one may be linked: %v", entries)
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

func TestSyncClaudeSkillLinks(t *testing.T) {
	root := t.TempDir()
	skills := layout.ResourceDir(root, "skills")
	claudeSkills := layout.ClaudeSkillsDir(root)
	write := func(p string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(skills, "mine", "SKILL.md"))
	write(filepath.Join(skills, "old-flat.md"))            // 옛 형식: 연결하지 않는다
	write(filepath.Join(claudeSkills, "user", "SKILL.md")) // 사용자 것: 남긴다
	if err := os.Symlink(layout.SkillLinkTarget("gone"), filepath.Join(claudeSkills, "gone")); err != nil {
		t.Fatal(err)
	}
	// 사용자 링크: pylon 스킬과 이름이 같아도, 없어도 남긴다
	write(filepath.Join(skills, "clash", "SKILL.md"))
	for _, name := range []string{"clash", "foreign"} {
		if err := os.Symlink(t.TempDir(), filepath.Join(claudeSkills, name)); err != nil {
			t.Fatal(err)
		}
	}

	if err := syncClaudeSkillLinks(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(claudeSkills, "mine", "SKILL.md")); err != nil {
		t.Errorf("skill not linked: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(claudeSkills, "old-flat.md")); err == nil {
		t.Error("flat .md must not be linked")
	}
	if _, err := os.Stat(filepath.Join(claudeSkills, "user", "SKILL.md")); err != nil {
		t.Error("user skill removed")
	}
	if _, err := os.Lstat(filepath.Join(claudeSkills, "gone")); err == nil {
		t.Error("stale link kept")
	}
	for _, name := range []string{"clash", "foreign"} {
		if target, err := os.Readlink(filepath.Join(claudeSkills, name)); err != nil || target == layout.SkillLinkTarget(name) {
			t.Errorf("user link %s changed: %q, %v", name, target, err)
		}
	}

	// .claude/skills 자체가 링크면 그 안에 쓰지 않는다
	root2, outside := t.TempDir(), t.TempDir()
	write(filepath.Join(layout.ResourceDir(root2, "skills"), "mine", "SKILL.md"))
	if err := os.MkdirAll(layout.ClaudeDir(root2), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, layout.ClaudeSkillsDir(root2)); err != nil {
		t.Fatal(err)
	}
	if err := syncClaudeSkillLinks(root2); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Errorf("wrote into linked skills dir: %v", entries)
	}
}
