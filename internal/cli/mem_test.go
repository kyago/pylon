package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/layout"
	"github.com/kyago/pylon/internal/memory"
)

const memInjectHeader = "## pylon 프로젝트 메모리\n"
const memInjectFooter = "각 줄 끝의 경로(`.pylon/memory/` 기준)가 전문 파일입니다. 같은 key로 `pylon mem store` 하면 갱신됩니다.\n"

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

func writeMemConfig(t *testing.T, root, memoryYAML string) {
	t.Helper()
	if err := os.WriteFile(layout.ConfigPath(root), []byte("version: \"0.1\"\nmemory:\n"+memoryYAML), 0644); err != nil {
		t.Fatal(err)
	}
}

func insertMem(t *testing.T, root, project, key, content string) {
	t.Helper()
	e := &memory.Entry{ProjectID: project, Category: "decision", Key: key, Content: content, Confidence: 0.8}
	if err := memory.NewStore(root).Insert(e); err != nil {
		t.Fatalf("Insert 실패: %v", err)
	}
}

// 프로젝트를 이름순으로 주입하고 헤더/꼬리말로 감싼다.
func TestMemInject_TwoProjects(t *testing.T) {
	root := setupTestWorkspace(t)
	insertMem(t, root, "beta", "beta-key", "beta 프로젝트의 결정 요약")
	insertMem(t, root, "alpha", "alpha-key", "alpha 프로젝트의 결정 요약")

	out := runMemInjectIn(t, root)
	if !strings.HasPrefix(out, memInjectHeader) || !strings.HasSuffix(out, memInjectFooter) {
		t.Fatalf("헤더/꼬리말로 감싸야 한다:\n%s", out)
	}
	a, b := strings.Index(out, "#### alpha"), strings.Index(out, "#### beta")
	if a < 0 || b < 0 || a > b {
		t.Errorf("alpha, beta 순으로 모두 주입되어야 한다:\n%s", out)
	}
}

// 앞 프로젝트가 예산을 거의 다 쓰면 남은 예산에 못 들어가는 프로젝트는 빠진다.
func TestMemInject_BudgetExceeded(t *testing.T) {
	root := setupTestWorkspace(t)
	insertMem(t, root, "alpha", "alpha-key", "alpha 프로젝트의 결정 요약")
	insertMem(t, root, "beta", "beta-key", "beta 프로젝트의 결정 요약")

	md, err := memory.NewStore(root).InjectionMarkdown("alpha", 0)
	if err != nil {
		t.Fatal(err)
	}
	// InjectionMarkdown은 생략 표기(12바이트) 자리를 남기므로 alpha 전체 + 12바이트를
	// 겨우 담는 예산이면 beta에는 15바이트 이하만 남는다.
	const ellipsisBytes = len("…(생략)\n")
	writeMemConfig(t, root, "  proactive_max_tokens: "+strconv.Itoa((len(md)+ellipsisBytes+3)/4)+"\n")

	out := runMemInjectIn(t, root)
	if !strings.Contains(out, "#### alpha") {
		t.Errorf("alpha는 예산 안에 들어가야 한다:\n%s", out)
	}
	if strings.Contains(out, "#### beta") {
		t.Errorf("예산을 넘는 beta는 주입되면 안 된다:\n%s", out)
	}
}

// InjectionMarkdown은 0 이하 예산을 무제한으로 취급하므로, 예산이 없으면
// 호출 자체를 하지 않아야 한다.
func TestMemInject_NonPositiveBudget(t *testing.T) {
	root := setupTestWorkspace(t)
	insertMem(t, root, "alpha", "alpha-key", "alpha 프로젝트의 결정 요약")
	writeMemConfig(t, root, "  proactive_max_tokens: -1\n")

	if out := runMemInjectIn(t, root); out != "" {
		t.Errorf("예산이 0 이하면 출력이 없어야 한다: %q", out)
	}
}

func TestMemInject_Disabled(t *testing.T) {
	root := setupTestWorkspace(t)
	insertMem(t, root, "alpha", "alpha-key", "alpha 프로젝트의 결정 요약")
	writeMemConfig(t, root, "  proactive_injection: false\n")

	if out := runMemInjectIn(t, root); out != "" {
		t.Errorf("proactive_injection=false면 출력이 없어야 한다: %q", out)
	}
}

func TestMemInject_NoMemory(t *testing.T) {
	root := setupTestWorkspace(t)
	if out := runMemInjectIn(t, root); out != "" {
		t.Errorf("메모리가 없으면 출력이 없어야 한다: %q", out)
	}
}

// 세션 시작 주입 전에 retention_days(기본 learning 30일)를 넘긴 항목을 정리한다.
func TestMemInject_PrunesExpired(t *testing.T) {
	root := setupTestWorkspace(t)
	store := memory.NewStore(root)
	old := &memory.Entry{ProjectID: "alpha", Category: "learning", Key: "오래된 학습",
		Content: "31일이 지나 만료 정리 대상인 오래된 학습", Confidence: 0.8,
		CreatedAt: time.Now().UTC().Add(-31 * 24 * time.Hour)}
	if err := store.Insert(old); err != nil {
		t.Fatal(err)
	}
	insertMem(t, root, "alpha", "새 결정", "아직 유효한 새 결정 요약")

	out := runMemInjectIn(t, root)
	if _, err := os.Stat(root + "/" + old.Path); !os.IsNotExist(err) {
		t.Errorf("만료 항목 파일이 삭제되어야 한다: %v", err)
	}
	if strings.Contains(out, "오래된 학습") || !strings.Contains(out, "새 결정") {
		t.Errorf("만료 항목은 빠지고 새 항목만 주입되어야 한다:\n%s", out)
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

// 구버전 Stop 훅이 저장한 훅 JSON 원문은 보존 정책을 꺼 둔 워크스페이스에서도 주입 전에 지운다.
func TestMemInject_RemovesLegacyHookEntries(t *testing.T) {
	root := setupTestWorkspace(t)
	writeMemConfig(t, root, "  retention_days: {}\n")
	junk := &memory.Entry{ProjectID: "alpha", Category: "learning", Author: "claude", Confidence: 0.8,
		Key:     `session-20261029-172146/{"session_id"-"8c1d2e3f`,
		Content: `{"session_id":"8c1d2e3f-0a9b-4c8d-b7e6-f5a4b3c2d1e0","transcript_path":"/x.jsonl","hook_event_name":"Stop","stop_hook_active":false}`}
	if err := memory.NewStore(root).Insert(junk); err != nil {
		t.Fatal(err)
	}
	insertMem(t, root, "alpha", "새 결정", "아직 유효한 새 결정 요약")

	out := runMemInjectIn(t, root)
	if _, err := os.Stat(root + "/" + junk.Path); !os.IsNotExist(err) {
		t.Errorf("구버전 훅 항목 파일이 삭제되어야 한다: %v", err)
	}
	if strings.Contains(out, "hook_event_name") || !strings.Contains(out, "새 결정") {
		t.Errorf("훅 JSON은 빠지고 실제 항목만 주입되어야 한다:\n%s", out)
	}
}

// staleClaudeWorkspace는 이전 버전 pylon이 만든 상태를 흉내 낸다: 생성 파일을 만든 뒤
// 매뉴얼과 AGENTS.md 블록을 낡게 하고, 이제 없는 내장 에이전트를 남긴다.
func staleClaudeWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(layout.PylonDir(root), 0o755); err != nil {
		t.Fatal(err)
	}
	writeMemConfig(t, root, "  proactive_injection: true\n  proactive_max_tokens: 2000\n")
	insertMem(t, root, "alpha", "k1", "내용")
	cfg, err := config.LoadConfig(layout.ConfigPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if err := generateClaudeDir(root, cfg, nil); err != nil {
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
	stale := strings.Replace(string(agents), "## 작업 진행 방식", "## 옛 제목", 1)
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte("# 내 규칙\n\n"+stale), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// 업데이트 뒤 첫 세션은 pylon이나 doctor 없이 워크스페이스를 갱신하고, stdout에는 주입 본문만 낸다.
func TestMemInject_RefreshesWorkspaceAfterUpgrade(t *testing.T) {
	root := staleClaudeWorkspace(t)

	out := runMemInjectIn(t, root)
	if !strings.HasPrefix(out, memInjectHeader) || !strings.HasSuffix(out, memInjectFooter) {
		t.Errorf("stdout에 주입 본문 외의 내용이 섞였다: %q", out)
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
	if !strings.HasPrefix(string(agents), "# 내 규칙\n\n") || !strings.Contains(string(agents), "## 작업 진행 방식") {
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

// 갱신이 실패해도 메모리는 주입되고, 원인을 고치면 다음 세션이 다시 시도한다.
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

	if out := runMemInjectIn(t, root); !strings.HasPrefix(out, memInjectHeader) {
		t.Errorf("갱신 실패가 메모리 주입을 막았다: %q", out)
	}
	if got, _ := os.ReadFile(layout.RootAgentsPath(root)); string(got) != broken {
		t.Errorf("깨진 AGENTS.md를 건드렸다: %q", got)
	}

	if err := os.WriteFile(layout.RootAgentsPath(root), good, 0o644); err != nil {
		t.Fatal(err)
	}
	runMemInjectIn(t, root)
	if got, _ := os.ReadFile(layout.RootAgentsPath(root)); !strings.Contains(string(got), "## 작업 진행 방식") {
		t.Errorf("다음 세션이 다시 시도하지 않았다: %.80q", got)
	}
}

// Claude Code를 쓰지 않는 워크스페이스에서 수동 실행해도 .claude/를 만들지 않는다.
func TestMemInject_DoesNotCreateClaudeDir(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(layout.PylonDir(root), 0o755); err != nil {
		t.Fatal(err)
	}
	writeMemConfig(t, root, "  proactive_injection: true\n")
	runMemInjectIn(t, root)
	if _, err := os.Stat(layout.ClaudeDir(root)); !os.IsNotExist(err) {
		t.Errorf(".claude/가 생겼다: %v", err)
	}
	if _, err := os.Stat(filepath.Join(layout.BuiltinResourceDir(root, "reference"), "pylon-usage.md")); err != nil {
		t.Errorf("내장 리소스가 갱신되지 않았다: %v", err)
	}
}
