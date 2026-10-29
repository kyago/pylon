package cli

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/kyago/pylon/internal/layout"
	"github.com/kyago/pylon/internal/memory"
)

const memInjectHeader = "## pylon 프로젝트 메모리\n"
const memInjectFooter = "전문은 .pylon/memory/<project>/<category>/<key>.md 를 읽고, 같은 key로 pylon mem store 하면 갱신됩니다.\n"

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
