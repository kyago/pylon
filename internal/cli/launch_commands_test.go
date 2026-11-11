package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kyago/pylon/internal/layout"
)

// 내장 에이전트 파일의 name: 은 파일명과 같아야 한다 — Claude Code가 .claude/agents/ 로
// 자동 등록할 때 매뉴얼·사용자가 부르는 이름과 어긋나지 않도록.
func TestBuiltinAgentNamesMatchFilenames(t *testing.T) {
	entries, err := embeddedAgents.ReadDir("agents")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("no embedded agents")
	}
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".md")
		data, err := embeddedAgents.ReadFile("agents/" + e.Name())
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "\nname: "+name+"\n") {
			t.Errorf("agents/%s: frontmatter name differs from filename", e.Name())
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
