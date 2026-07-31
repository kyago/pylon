package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kyago/pylon/internal/config"
)

// 소유권 계약: 내장 리소스와 같은 이름의 파일은 pylon 소유이므로 내장 버전으로 되돌아간다.
func TestSyncPylonResources_RevertsShippedFiles(t *testing.T) {
	pylonDir := filepath.Join(t.TempDir(), ".pylon")
	cmdsDir := filepath.Join(pylonDir, "commands")
	if err := os.MkdirAll(cmdsDir, 0755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(cmdsDir, "pl-verify.md")
	if err := os.WriteFile(stale, []byte("OLD VERSION\n"), 0644); err != nil {
		t.Fatal(err)
	}

	written, overwritten := syncPylonResources(pylonDir)
	if written == 0 {
		t.Fatal("내장 리소스가 설치되어야 한다")
	}

	got, err := os.ReadFile(stale)
	if err != nil {
		t.Fatal(err)
	}
	want, err := embeddedCommands.ReadFile("commands/pl-verify.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatal("pylon 소유 파일이 내장 버전으로 되돌아가지 않았다")
	}
	if len(overwritten) != 1 || overwritten[0] != "commands/pl-verify.md" {
		t.Fatalf("되돌린 파일이 이름으로 보고되어야 한다: %v", overwritten)
	}
}

// 소유권 계약: 내장 리소스에 없는 이름의 파일은 사용자 소유이므로 건드리지 않는다
// (`pylon add-agent`/`add-skill`로 만든 리소스가 여기 해당한다).
func TestSyncPylonResources_LeavesUserAuthoredFilesAlone(t *testing.T) {
	pylonDir := filepath.Join(t.TempDir(), ".pylon")
	agentsDir := filepath.Join(pylonDir, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatal(err)
	}
	userAgent := filepath.Join(agentsDir, "my-own-agent.md")
	if err := os.WriteFile(userAgent, []byte("# 내가 만든 에이전트\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if _, overwritten := syncPylonResources(pylonDir); len(overwritten) != 0 {
		t.Fatalf("사용자 파일은 되돌림 대상이 아니다: %v", overwritten)
	}

	got, err := os.ReadFile(userAgent)
	if err != nil {
		t.Fatalf("사용자 파일이 삭제되었다: %v", err)
	}
	if string(got) != "# 내가 만든 에이전트\n" {
		t.Fatalf("사용자 파일이 수정되었다: %q", got)
	}
}

// 새 릴리스에서 바뀐 커맨드는 이번 실행에서 바로 .claude/commands/까지 반영되어야 한다
// (리소스 동기화가 desired 계산보다 먼저 돌아야 한다는 순서 계약).
func TestGenerateClaudeDir_PropagatesRefreshedCommand(t *testing.T) {
	root := t.TempDir()
	pylonCmds := filepath.Join(root, ".pylon", "commands")
	if err := os.MkdirAll(pylonCmds, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pylonCmds, "pl-pipeline.md"), []byte("OLD VERSION\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := generateClaudeDir(root, &config.Config{}, nil); err != nil {
		t.Fatal(err)
	}

	claudeCmds := filepath.Join(root, ".claude", "commands", "pl")
	// 워크스페이스에 없던 커맨드도 같은 실행에서 설치·반영된다.
	if _, err := os.Stat(filepath.Join(claudeCmds, "verify.md")); err != nil {
		t.Fatalf("새 커맨드가 같은 실행에서 반영되지 않았다: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(claudeCmds, "pipeline.md"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := embeddedCommands.ReadFile("commands/pl-pipeline.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatal("낡은 커맨드가 같은 실행에서 최신 내용으로 반영되지 않았다")
	}
}
