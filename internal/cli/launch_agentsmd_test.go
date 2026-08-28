package cli

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/layout"
)

func TestBuildClaudeMDPointerIsImportMarker(t *testing.T) {
	if got := buildClaudeMDPointer(); strings.TrimSpace(got) != "@AGENTS.md" {
		t.Errorf("pointer should be the @AGENTS.md import marker, got %q", got)
	}
}

func TestBuildAgentsBlockHasMarkersStampAndInstruction(t *testing.T) {
	out := buildAgentsBlock(t.TempDir(), []config.ProjectInfo{{Name: "api"}})
	if !strings.HasPrefix(out, agentsBlockBegin+"\n") || !strings.HasSuffix(out, agentsBlockEnd+"\n") {
		t.Errorf("block must be wrapped in markers: %s", out)
	}
	if !strings.Contains(out, fmt.Sprintf("pylon-usage-version: %d", pylonUsageVersion)) {
		t.Errorf("block missing version stamp: %s", out)
	}
	if !strings.Contains(out, ".pylon/reference/pylon-usage.md") {
		t.Errorf("block must point at the manual: %s", out)
	}
	if !strings.Contains(out, "api") {
		t.Errorf("block should list project facts: %s", out)
	}
}

func TestAgentsMDStale(t *testing.T) {
	root := t.TempDir()
	// (1) 파일 없음 → stale
	if !agentsMDStale(root) {
		t.Error("missing AGENTS.md should be stale")
	}
	// (2) 낮은 버전 스탬프의 블록 → stale
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte(agentsBlockBegin+"\n<!-- pylon-usage-version: 0 -->\n# guide\n"+agentsBlockEnd+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if !agentsMDStale(root) {
		t.Error("lower version stamp should be stale")
	}
	// (3) 블록 없음 → stale (스탬프가 블록 밖에 있어도 마찬가지)
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte(fmt.Sprintf("<!-- pylon-usage-version: %d -->\n# no block", pylonUsageVersion)), 0644); err != nil {
		t.Fatal(err)
	}
	if !agentsMDStale(root) {
		t.Error("missing block should be stale even with an outside stamp")
	}
	// (4) 현재 버전 블록 → not stale (블록 밖 사용자 내용과 무관)
	current := "# 사용자 규칙\n\n" + agentsBlockBegin + fmt.Sprintf("\n<!-- pylon-usage-version: %d -->\n# guide\n", pylonUsageVersion) + agentsBlockEnd + "\n"
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte(current), 0644); err != nil {
		t.Fatal(err)
	}
	if agentsMDStale(root) {
		t.Error("current version stamp should not be stale")
	}
}

func TestEnsureRootAgentFilesWritesPointerAndBootstrapWhenMissing(t *testing.T) {
	root := t.TempDir()
	bootstrapped, _, err := ensureRootAgentFiles(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bootstrapped {
		t.Error("missing AGENTS.md should be bootstrapped")
	}
	ptr, _ := os.ReadFile(layout.RootClaudePath(root))
	if strings.TrimSpace(string(ptr)) != "@AGENTS.md" {
		t.Errorf("CLAUDE.md should be the import marker, got %q", ptr)
	}
	if _, err := os.Stat(layout.RootAgentsPath(root)); err != nil {
		t.Errorf("AGENTS.md bootstrap not written: %v", err)
	}
}

func TestEnsureRootAgentFilesAppendsBlockToUserAgentsMD(t *testing.T) {
	root := t.TempDir()
	handAgents := "# 우리 팀 가이드\n스탬프 없는 수작업 파일"
	handClaude := "# 우리 팀 CLAUDE.md\n마커가 아닌 수작업 파일"
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte(handAgents), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(layout.RootClaudePath(root), []byte(handClaude), 0644); err != nil {
		t.Fatal(err)
	}

	bootstrapped, backedUp, err := ensureRootAgentFiles(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bootstrapped {
		t.Error("user AGENTS.md without a block should get the block appended")
	}

	// 사용자 AGENTS.md는 백업이 아니라 그 자리에 보존되고, 블록이 뒤에 붙는다.
	nowAgents, _ := os.ReadFile(layout.RootAgentsPath(root))
	if !strings.HasPrefix(string(nowAgents), handAgents) {
		t.Errorf("user AGENTS.md content must be preserved in place: %q", nowAgents)
	}
	if !strings.Contains(string(nowAgents), agentsBlockBegin) || !strings.Contains(string(nowAgents), agentsBlockEnd) {
		t.Errorf("pylon block not appended: %q", nowAgents)
	}
	for _, name := range backedUp {
		if name == "AGENTS.md" {
			t.Errorf("user AGENTS.md must not be backed up/moved aside, got %v", backedUp)
		}
	}

	// CLAUDE.md는 여전히 백업 후 마커로 교체된다.
	gotClaude, err := os.ReadFile(layout.RootClaudePath(root) + rootFileBackupSuffix)
	if err != nil {
		t.Fatalf("CLAUDE.md backup missing: %v", err)
	}
	if string(gotClaude) != handClaude {
		t.Errorf("CLAUDE.md backup content differs: %q", gotClaude)
	}
	nowClaude, _ := os.ReadFile(layout.RootClaudePath(root))
	if strings.TrimSpace(string(nowClaude)) != "@AGENTS.md" {
		t.Errorf("CLAUDE.md should be the marker, got %q", nowClaude)
	}
}

func TestEnsureRootAgentFilesIsIdempotent(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte("# hand\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(layout.RootClaudePath(root), []byte("# hand"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ensureRootAgentFiles(root, nil); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(layout.RootAgentsPath(root))

	// 2회차: 블록이 이미 최신이므로 아무것도 바뀌지 않는다.
	bootstrapped, backedUp, err := ensureRootAgentFiles(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if bootstrapped {
		t.Error("second run must not re-bootstrap a current block")
	}
	if len(backedUp) != 0 {
		t.Errorf("second run must not back anything up, got %v", backedUp)
	}
	second, _ := os.ReadFile(layout.RootAgentsPath(root))
	if string(second) != string(first) {
		t.Errorf("AGENTS.md changed on idempotent re-run: %q vs %q", second, first)
	}
	claudeBak := layout.RootClaudePath(root) + rootFileBackupSuffix
	if got, _ := os.ReadFile(claudeBak); string(got) != "# hand" {
		t.Errorf("pylon-authored CLAUDE.md must not be re-backed-up: %q", got)
	}
}

// 버전 스탬프를 올렸을 때, 세션이 저작한 가이드는 stale이지만 버려서는 안 된다.
func TestEnsureRootAgentFilesBacksUpStaleAuthoredAgentsMD(t *testing.T) {
	root := t.TempDir()
	authored := "<!-- pylon-usage-version: 0 -->\n# 세션이 저작한 소중한 가이드\n상세 운영 규칙"
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte(authored), 0644); err != nil {
		t.Fatal(err)
	}
	bootstrapped, backedUp, err := ensureRootAgentFiles(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bootstrapped {
		t.Error("stale AGENTS.md should be re-bootstrapped")
	}
	if len(backedUp) != 1 || backedUp[0] != "AGENTS.md" {
		t.Errorf("stale authored AGENTS.md should be reported as backed up, got %v", backedUp)
	}
	got, err := os.ReadFile(layout.RootAgentsPath(root) + rootFileBackupSuffix)
	if err != nil {
		t.Fatalf("authored guide destroyed without backup: %v", err)
	}
	if string(got) != authored {
		t.Errorf("backup content differs: %q", got)
	}
}

// stale 블록은 블록만 교체된다 — 블록 밖 사용자 내용은 백업 없이 그 자리에 보존.
func TestEnsureRootAgentFilesReplacesStaleBlockPreservingUserContent(t *testing.T) {
	root := t.TempDir()
	userTop := "# 우리 팀 규칙\n블록 밖 내용\n\n"
	userBottom := "\n# 블록 아래 추가 규칙\n"
	staleBlock := strings.Replace(buildAgentsBlock(root, nil),
		fmt.Sprintf("pylon-usage-version: %d", pylonUsageVersion), "pylon-usage-version: 0", 1)
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte(userTop+staleBlock+userBottom), 0644); err != nil {
		t.Fatal(err)
	}
	bootstrapped, backedUp, err := ensureRootAgentFiles(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bootstrapped {
		t.Error("stale block should be re-bootstrapped")
	}
	if len(backedUp) != 0 {
		t.Errorf("block replacement must not back anything up, got %v", backedUp)
	}
	got, _ := os.ReadFile(layout.RootAgentsPath(root))
	if !strings.HasPrefix(string(got), userTop) || !strings.HasSuffix(string(got), userBottom) {
		t.Errorf("user content around the block was not preserved: %q", got)
	}
	if !strings.Contains(string(got), fmt.Sprintf("pylon-usage-version: %d", pylonUsageVersion)) {
		t.Errorf("block not refreshed to current stamp: %q", got)
	}
	if strings.Count(string(got), agentsBlockBegin) != 1 || strings.Count(string(got), agentsBlockEnd) != 1 {
		t.Errorf("block replacement must stay a single block: %q", got)
	}
}

// PR 이전 워크스페이스의 생성된 CLAUDE.md(하드코딩 프롬프트)는 pylon 산출물이므로
// 백업 없이 마커로 교체되어야 한다 — 아니면 업그레이드마다 .pylon-bak이 생긴다.
func TestEnsureRootAgentFilesReplacesLegacyGeneratedClaudeMD(t *testing.T) {
	root := t.TempDir()
	legacy := legacyClaudeMDHeading + "\n\n당신은 Pylon 워크스페이스의 루트 에이전트입니다.\n"
	if err := os.WriteFile(layout.RootClaudePath(root), []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}
	_, backedUp, err := ensureRootAgentFiles(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(backedUp) != 0 {
		t.Errorf("legacy generated CLAUDE.md should be replaced silently, got %v", backedUp)
	}
	got, _ := os.ReadFile(layout.RootClaudePath(root))
	if strings.TrimSpace(string(got)) != "@AGENTS.md" {
		t.Errorf("CLAUDE.md should be the marker, got %q", got)
	}
}

// 백업이 두 번 필요하면 이전 백업을 덮어쓰지 않는다.
func TestBackupIfHandWrittenDoesNotClobber(t *testing.T) {
	root := t.TempDir()
	path := layout.RootAgentsPath(root)
	never := func([]byte) bool { return false }

	if err := os.WriteFile(path, []byte("첫 번째"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := backupIfHandWritten(path, never); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("두 번째"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := backupIfHandWritten(path, never); err != nil {
		t.Fatal(err)
	}

	if got, _ := os.ReadFile(path + rootFileBackupSuffix); string(got) != "첫 번째" {
		t.Errorf("first backup was clobbered: %q", got)
	}
	if got, _ := os.ReadFile(path + rootFileBackupSuffix + ".1"); string(got) != "두 번째" {
		t.Errorf("second backup not written to a free name: %q", got)
	}
}

func TestEnsureRootAgentFilesPreservesFreshAgentsMD(t *testing.T) {
	root := t.TempDir()
	authored := "# 사용자 규칙\n\n" + agentsBlockBegin +
		fmt.Sprintf("\n<!-- pylon-usage-version: %d -->\n# 세션이 저작한 가이드\n", pylonUsageVersion) +
		agentsBlockEnd + "\n"
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte(authored), 0644); err != nil {
		t.Fatal(err)
	}
	bootstrapped, _, err := ensureRootAgentFiles(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if bootstrapped {
		t.Error("fresh AGENTS.md must not be bootstrapped over")
	}
	got, _ := os.ReadFile(layout.RootAgentsPath(root))
	if string(got) != authored {
		t.Errorf("fresh AGENTS.md was overwritten: %q", got)
	}
}
