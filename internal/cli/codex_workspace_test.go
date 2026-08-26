package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kyago/pylon/internal/layout"
)

func newCodexSkillsWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	commandsDir := layout.CommandsDir(root)
	if err := os.MkdirAll(commandsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	command := "---\ndescription: \"빌드/테스트/린트 검증 실행\"\n---\n\n# Verification\n본문\n"
	if err := os.WriteFile(filepath.Join(commandsDir, "pl-verify.md"), []byte(command), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestGenerateCodexSkills_CreatesPointerSkill(t *testing.T) {
	root := newCodexSkillsWorkspace(t)
	if err := generateCodexSkills(root); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(layout.CodexSkillsDir(root), "pl-verify", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	skill := string(data)
	for _, want := range []string{
		"name: pl-verify",
		"description: 빌드/테스트/린트 검증 실행",
		codexSkillMarker,
		".pylon/commands/pl-verify.md",
	} {
		if !strings.Contains(skill, want) {
			t.Fatalf("skill missing %q:\n%s", want, skill)
		}
	}
}

func TestGenerateCodexSkills_PrunesStaleGeneratedSkill(t *testing.T) {
	root := newCodexSkillsWorkspace(t)
	staleDir := filepath.Join(layout.CodexSkillsDir(root), "pl-removed")
	if err := os.MkdirAll(staleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := "---\nname: pl-removed\n---\n" + codexSkillMarker + "\n"
	if err := os.WriteFile(filepath.Join(staleDir, "SKILL.md"), []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := generateCodexSkills(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(staleDir); !os.IsNotExist(err) {
		t.Fatal("stale generated skill must be pruned")
	}
}

func TestGenerateCodexSkills_NeverTouchesUserSkills(t *testing.T) {
	root := newCodexSkillsWorkspace(t)
	userDir := filepath.Join(layout.CodexSkillsDir(root), "my-skill")
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatal(err)
	}
	userSkill := "---\nname: my-skill\ndescription: mine\n---\n사용자 스킬\n"
	if err := os.WriteFile(filepath.Join(userDir, "SKILL.md"), []byte(userSkill), 0o644); err != nil {
		t.Fatal(err)
	}
	// 이름이 커맨드와 겹치는 사용자 스킬도 덮어쓰지 않는다.
	conflictDir := filepath.Join(layout.CodexSkillsDir(root), "pl-verify")
	if err := os.MkdirAll(conflictDir, 0o755); err != nil {
		t.Fatal(err)
	}
	conflict := "---\nname: pl-verify\ndescription: 사용자 커스텀\n---\n커스텀 본문\n"
	if err := os.WriteFile(filepath.Join(conflictDir, "SKILL.md"), []byte(conflict), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := generateCodexSkills(root); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(userDir, "SKILL.md")); string(data) != userSkill {
		t.Fatal("user skill must be untouched")
	}
	if data, _ := os.ReadFile(filepath.Join(conflictDir, "SKILL.md")); string(data) != conflict {
		t.Fatal("user skill shadowing a command name must not be overwritten")
	}
}

func TestGenerateCodexSkills_IdempotentAndRefreshesStaleContent(t *testing.T) {
	root := newCodexSkillsWorkspace(t)
	if err := generateCodexSkills(root); err != nil {
		t.Fatal(err)
	}
	skillPath := filepath.Join(layout.CodexSkillsDir(root), "pl-verify", "SKILL.md")
	first, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	// 마커를 유지한 채 내용이 구버전이면 갱신된다.
	if err := os.WriteFile(skillPath, []byte("---\nname: pl-verify\n---\n"+codexSkillMarker+"\nold\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := generateCodexSkills(root); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("stale generated skill must be refreshed to current content:\n%s", second)
	}
}
