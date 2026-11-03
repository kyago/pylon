package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kyago/pylon/internal/layout"
)

// 이전 릴리스가 만든 포인터 스킬(마커 보유)만 지우고 사용자 스킬은 건드리지 않는다.
func TestPruneCodexSkills_RemovesOnlyGenerated(t *testing.T) {
	root := t.TempDir()
	skills := layout.CodexSkillsDir(root)
	write := func(name, body string) {
		t.Helper()
		dir := filepath.Join(skills, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("pl-pipeline", "---\nname: pl-pipeline\n---\n"+codexSkillMarker+"\n")
	write("my-skill", "---\nname: my-skill\n---\nmine\n")

	pruneCodexSkills(root)

	if _, err := os.Stat(filepath.Join(skills, "pl-pipeline")); !os.IsNotExist(err) {
		t.Errorf("generated skill survived: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(skills, "my-skill", "SKILL.md"))
	if err != nil || !strings.Contains(string(got), "mine") {
		t.Errorf("user skill touched: %q, %v", got, err)
	}
}
