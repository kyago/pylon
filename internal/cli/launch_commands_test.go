package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kyago/pylon/internal/layout"
)

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
