package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kyago/pylon/internal/layout"
	"github.com/kyago/pylon/internal/memory"
)

// runMemInjectIn runs `pylon mem inject` against root and returns its stdout.
func runMemInjectIn(t *testing.T, root string) string {
	t.Helper()
	prev := flagWorkspace
	flagWorkspace = root
	t.Cleanup(func() { flagWorkspace = prev })
	var err error
	out := captureStdout(t, func() { err = runMemInject() })
	if err != nil {
		t.Fatalf("runMemInject 실패: %v", err)
	}
	return out
}

func insertMem(t *testing.T, root, project, key, content string) {
	t.Helper()
	e := &memory.Entry{ProjectID: project, Category: "decision", Key: key, Content: content, Confidence: 0.8}
	if err := memory.NewStore(root).Insert(e); err != nil {
		t.Fatalf("Insert 실패: %v", err)
	}
}

// 같은 key 재저장은 "갱신됨"과 JSON "updated": true로 알린다.
func TestMemStore_ReportsUpdate(t *testing.T) {
	root := setupTestWorkspace(t)
	prev := flagWorkspace
	flagWorkspace = root
	t.Cleanup(func() { flagWorkspace = prev })

	store := func(content string) string {
		var err error
		out := captureStdout(t, func() { err = runMemStore("app", "같은 키", content, "decision", "agent", 0.8) })
		if err != nil {
			t.Fatalf("runMemStore 실패: %v", err)
		}
		return out
	}
	if out := store("처음 내용"); !strings.Contains(out, "저장됨") {
		t.Errorf("신규는 저장됨이어야 한다: %q", out)
	}
	if out := store("바뀐 내용"); !strings.Contains(out, "갱신됨") {
		t.Errorf("같은 key 재저장은 갱신됨이어야 한다: %q", out)
	}

	prevJSON := flagJSON
	flagJSON = true
	t.Cleanup(func() { flagJSON = prevJSON })
	if out := store("또 바뀐 내용"); !strings.Contains(out, `"updated":true`) {
		t.Errorf("JSON에 updated:true가 있어야 한다: %q", out)
	}
}

// 구버전 Stop 훅이 저장한 훅 JSON 원문은 옛 훅이 마지막으로 돌 때 지운다.
func TestMemInject_RemovesLegacyHookEntries(t *testing.T) {
	root := setupTestWorkspace(t)
	junk := &memory.Entry{ProjectID: "alpha", Category: "learning", Author: "claude", Confidence: 0.8,
		Key:     `session-20261029-172146/{"session_id"-"8c1d2e3f`,
		Content: `{"session_id":"8c1d2e3f-0a9b-4c8d-b7e6-f5a4b3c2d1e0","transcript_path":"/x.jsonl","hook_event_name":"Stop","stop_hook_active":false}`}
	if err := memory.NewStore(root).Insert(junk); err != nil {
		t.Fatal(err)
	}
	insertMem(t, root, "alpha", "새 결정", "아직 유효한 새 결정 요약")

	if out := runMemInjectIn(t, root); out != "" {
		t.Errorf("stdout은 비어 있어야 한다: %q", out)
	}
	entries, err := memory.NewStore(root).List("alpha")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Key == junk.Key {
		t.Errorf("훅 JSON은 빠지고 실제 항목만 남아야 한다: %+v", entries)
	}
}

// staleClaudeWorkspace는 이전 버전 pylon이 만든 상태를 흉내 낸다: 생성 파일을 만든 뒤
// 매뉴얼과 AGENTS.md 블록을 낡게 하고, 이제 없는 내장 에이전트를 남긴다.
func staleClaudeWorkspace(t *testing.T) string {
	t.Helper()
	root := setupTestWorkspace(t)
	insertMem(t, root, "alpha", "k1", "내용")
	if err := generateClaudeDir(root, nil); err != nil {
		t.Fatal(err)
	}
	settings := `{"hooks":{"SessionStart":[{"matcher":"startup","hooks":[{"type":"command","command":"pylon mem inject"}]}]}}`
	if err := os.WriteFile(filepath.Join(layout.ClaudeDir(root), "settings.json"), []byte(settings), 0o644); err != nil {
		t.Fatal(err)
	}
	manual := filepath.Join(layout.BuiltinResourceDir(root, "reference"), "pylon-usage.md")
	if err := os.WriteFile(manual, []byte("old manual"), 0o644); err != nil {
		t.Fatal(err)
	}
	retired := filepath.Join(layout.BuiltinResourceDir(root, "agents"), "retired.md")
	if err := os.WriteFile(retired, []byte("---\nname: retired\nrole: Old\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	agents, err := os.ReadFile(layout.RootAgentsPath(root))
	if err != nil {
		t.Fatal(err)
	}
	stale := strings.Replace(string(agents), "## 필요할 때 찾기", "## 옛 제목", 1)
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte("# 내 규칙\n\n"+stale), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// 옛 훅이 남은 워크스페이스는 첫 세션에서 스스로 갱신되고 훅이 지워지며, stdout에는 아무것도 내지 않는다.
func TestMemInject_RefreshesWorkspaceAfterUpgrade(t *testing.T) {
	root := staleClaudeWorkspace(t)

	if out := runMemInjectIn(t, root); out != "" {
		t.Errorf("훅의 stdout은 세션 컨텍스트가 되므로 비어 있어야 한다: %q", out)
	}
	if data, _ := os.ReadFile(filepath.Join(layout.ClaudeDir(root), "settings.json")); strings.Contains(string(data), "pylon mem inject") {
		t.Errorf("옛 훅이 남았다: %s", data)
	}
	manual, _ := os.ReadFile(filepath.Join(layout.BuiltinResourceDir(root, "reference"), "pylon-usage.md"))
	if string(manual) == "old manual" {
		t.Error("내장 매뉴얼이 갱신되지 않았다")
	}
	if _, err := os.Stat(filepath.Join(layout.BuiltinResourceDir(root, "agents"), "retired.md")); !os.IsNotExist(err) {
		t.Error("없어진 내장 에이전트가 남았다")
	}
	if _, err := os.Lstat(filepath.Join(layout.ClaudeAgentsDir(root), "retired.md")); !os.IsNotExist(err) {
		t.Error("없어진 에이전트의 .claude/agents 링크가 남았다")
	}
	agents, _ := os.ReadFile(layout.RootAgentsPath(root))
	if !strings.HasPrefix(string(agents), "# 내 규칙\n\n") || !strings.Contains(string(agents), "## 필요할 때 찾기") {
		t.Errorf("AGENTS.md 갱신 또는 사용자 내용 보존 실패: %.80q", agents)
	}
}

// 최신 상태에서는 파일을 다시 쓰지 않는다 — 세션마다 mtime을 바꿔 파일 워처를 깨우지 않기 위해.
func TestMemInject_NoWritesWhenUpToDate(t *testing.T) {
	root := staleClaudeWorkspace(t)
	runMemInjectIn(t, root)

	settings := filepath.Join(layout.ClaudeDir(root), "settings.json")
	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(settings, old, old); err != nil {
		t.Fatal(err)
	}
	runMemInjectIn(t, root)
	info, err := os.Stat(settings)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(old) {
		t.Errorf("최신 상태인데 settings.json을 다시 썼다: %v", info.ModTime())
	}
}

// 갱신이 실패해도 세션을 막지 않고, 원인을 고치면 다음 세션이 다시 시도한다.
func TestMemInject_RetriesRefreshAfterFailure(t *testing.T) {
	root := staleClaudeWorkspace(t)
	good, err := os.ReadFile(layout.RootAgentsPath(root))
	if err != nil {
		t.Fatal(err)
	}
	broken := agentsBlockBegin + "\n마커가 깨진 블록\n"
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}

	if out := runMemInjectIn(t, root); out != "" {
		t.Errorf("갱신 실패도 stdout으로 내보내면 안 된다: %q", out)
	}
	if got, _ := os.ReadFile(layout.RootAgentsPath(root)); string(got) != broken {
		t.Errorf("깨진 AGENTS.md를 건드렸다: %q", got)
	}

	if err := os.WriteFile(layout.RootAgentsPath(root), good, 0o644); err != nil {
		t.Fatal(err)
	}
	runMemInjectIn(t, root)
	if got, _ := os.ReadFile(layout.RootAgentsPath(root)); !strings.Contains(string(got), "## 필요할 때 찾기") {
		t.Errorf("다음 세션이 다시 시도하지 않았다: %.80q", got)
	}
}

// Claude Code를 쓰지 않는 워크스페이스에서 수동 실행해도 .claude/를 만들지 않는다.
func TestMemInject_DoesNotCreateClaudeDir(t *testing.T) {
	root := setupTestWorkspace(t)
	runMemInjectIn(t, root)
	if _, err := os.Stat(layout.ClaudeDir(root)); !os.IsNotExist(err) {
		t.Errorf(".claude/가 생겼다: %v", err)
	}
	if _, err := os.Stat(filepath.Join(layout.BuiltinResourceDir(root, "reference"), "pylon-usage.md")); err != nil {
		t.Errorf("내장 리소스가 갱신되지 않았다: %v", err)
	}
}
