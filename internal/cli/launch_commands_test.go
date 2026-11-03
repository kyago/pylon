package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/kyago/pylon/internal/layout"
)

// 매뉴얼 서브 에이전트 표가 부르는 이름은 모두 내장 에이전트 파일로 존재하고
// 그 name: 과 같아야 한다 — 에이전트를 통합·삭제하면서 남은 옛 이름이 호출되지 않도록.
func TestAgentReferencesResolveToBuiltins(t *testing.T) {
	refs := map[string]string{} // 이름 → 처음 발견한 위치
	manual, err := embeddedReference.ReadFile("reference/pylon-usage.md")
	if err != nil {
		t.Fatal(err)
	}
	section := string(manual)
	start := strings.Index(section, "### 서브 에이전트 선택 기준")
	if start < 0 {
		t.Fatal("manual agent selection section missing")
	}
	section = section[start:]
	if end := strings.Index(section, "\n## "); end >= 0 {
		section = section[:end]
	}
	backtick := regexp.MustCompile("`([^`]+)`")
	found := 0
	for _, line := range strings.Split(section, "\n") {
		cells := strings.Split(line, "|")
		if !strings.HasPrefix(line, "|") || len(cells) < 3 {
			continue
		}
		for _, m := range backtick.FindAllStringSubmatch(cells[2], -1) {
			refs[m[1]] = "reference/pylon-usage.md"
			found++
		}
	}
	if found < 3 {
		t.Fatalf("manual table agent names found = %d, parser likely broken", found)
	}

	for name, where := range refs {
		data, err := embeddedAgents.ReadFile("agents/" + name + ".md")
		if err != nil {
			t.Errorf("%s references agent %q with no agents/%s.md", where, name, name)
			continue
		}
		if !strings.Contains(string(data), "\nname: "+name+"\n") {
			t.Errorf("agents/%s.md: frontmatter name differs (referenced from %s)", name, where)
		}
	}
}

// 내장 이름의 사용자 커맨드는 내장 커맨드를 가리지 못하고, 사용자 파일은 그대로 남는다.
func TestGenerateClaudeDir_UserCommandNeverShadowsBuiltin(t *testing.T) {
	root := t.TempDir()
	pylonCmds := layout.ResourceDir(root, "commands")
	if err := os.MkdirAll(pylonCmds, 0755); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(pylonCmds, "pl-cleanup.md")
	if err := os.WriteFile(legacy, []byte("OLD VERSION\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := generateClaudeDir(root, nil); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(root, ".claude", "commands", "pl", "cleanup.md"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := embeddedCommands.ReadFile("commands/pl-cleanup.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatal("낡은 사본이 내장 커맨드를 가렸다")
	}
	if kept, err := os.ReadFile(legacy); err != nil || string(kept) != "OLD VERSION\n" {
		t.Fatalf("사용자 파일을 건드렸다: %q, %v", kept, err)
	}
}

// pl-cleanup 명령이 임베드되고 같은 실행에서 .claude/commands/pl/cleanup.md로 생성되어야 한다.
func TestGenerateClaudeDir_InstallsCleanupCommand(t *testing.T) {
	root := t.TempDir()

	if err := generateClaudeDir(root, nil); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(root, ".claude", "commands", "pl", "cleanup.md"))
	if err != nil {
		t.Fatalf("pl:cleanup 명령이 생성되지 않았다: %v", err)
	}
	want, err := embeddedCommands.ReadFile("commands/pl-cleanup.md")
	if err != nil {
		t.Fatalf("pl-cleanup.md가 임베드되지 않았다: %v", err)
	}
	if string(got) != string(want) {
		t.Fatal("생성된 cleanup 명령이 임베드 원본과 일치하지 않는다")
	}
}
