package cli

import (
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

func TestBuildBootstrapAgentsMDHasStampAndInstruction(t *testing.T) {
	out := buildBootstrapAgentsMD(t.TempDir(), []config.ProjectInfo{{Name: "api"}})
	if !strings.Contains(out, "pylon-usage-version: 1") {
		t.Errorf("bootstrap missing version stamp: %s", out)
	}
	if !strings.Contains(out, ".pylon/reference/pylon-usage.md") {
		t.Errorf("bootstrap must point at the manual: %s", out)
	}
	if !strings.Contains(out, "AGENTS.md") {
		t.Errorf("bootstrap must instruct authoring AGENTS.md: %s", out)
	}
	if !strings.Contains(out, "api") {
		t.Errorf("bootstrap should list project facts: %s", out)
	}
}

func TestAgentsMDStale(t *testing.T) {
	root := t.TempDir()
	// (1) 파일 없음 → stale
	if !agentsMDStale(root) {
		t.Error("missing AGENTS.md should be stale")
	}
	// (2) 낮은 버전 스탬프 → stale
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte("<!-- pylon-usage-version: 0 -->\n# guide"), 0644); err != nil {
		t.Fatal(err)
	}
	if !agentsMDStale(root) {
		t.Error("lower version stamp should be stale")
	}
	// (3) 스탬프 없음 → stale
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte("# hand-written, no stamp"), 0644); err != nil {
		t.Fatal(err)
	}
	if !agentsMDStale(root) {
		t.Error("missing stamp should be stale")
	}
	// (4) 현재 버전 → not stale
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte("<!-- pylon-usage-version: 1 -->\n# guide"), 0644); err != nil {
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

func TestEnsureRootAgentFilesBacksUpHandWrittenFiles(t *testing.T) {
	root := t.TempDir()
	handAgents := "# 우리 팀 가이드\n스탬프 없는 수작업 파일"
	handClaude := "# 우리 팀 CLAUDE.md\n마커가 아닌 수작업 파일"
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte(handAgents), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(layout.RootClaudePath(root), []byte(handClaude), 0644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := ensureRootAgentFiles(root, nil); err != nil {
		t.Fatal(err)
	}

	// 원본은 백업으로 보존되어야 한다 — 조용한 데이터 손실 금지.
	gotAgents, err := os.ReadFile(layout.RootAgentsPath(root) + rootFileBackupSuffix)
	if err != nil {
		t.Fatalf("AGENTS.md backup missing: %v", err)
	}
	if string(gotAgents) != handAgents {
		t.Errorf("AGENTS.md backup content differs: %q", gotAgents)
	}
	gotClaude, err := os.ReadFile(layout.RootClaudePath(root) + rootFileBackupSuffix)
	if err != nil {
		t.Fatalf("CLAUDE.md backup missing: %v", err)
	}
	if string(gotClaude) != handClaude {
		t.Errorf("CLAUDE.md backup content differs: %q", gotClaude)
	}

	// 그리고 원래 자리에는 pylon이 관리하는 내용이 들어간다.
	nowClaude, _ := os.ReadFile(layout.RootClaudePath(root))
	if strings.TrimSpace(string(nowClaude)) != "@AGENTS.md" {
		t.Errorf("CLAUDE.md should be the marker, got %q", nowClaude)
	}
	nowAgents, _ := os.ReadFile(layout.RootAgentsPath(root))
	if !strings.Contains(string(nowAgents), "pylon-usage-version: 1") {
		t.Errorf("AGENTS.md should be bootstrapped, got %.60q", nowAgents)
	}
}

func TestEnsureRootAgentFilesDoesNotBackUpPylonAuthoredFiles(t *testing.T) {
	root := t.TempDir()
	// 1회차: 수작업 파일이 백업된다.
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte("# hand"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(layout.RootClaudePath(root), []byte("# hand"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ensureRootAgentFiles(root, nil); err != nil {
		t.Fatal(err)
	}
	agentsBak := layout.RootAgentsPath(root) + rootFileBackupSuffix
	claudeBak := layout.RootClaudePath(root) + rootFileBackupSuffix
	first, err := os.ReadFile(agentsBak)
	if err != nil {
		t.Fatalf("first run should back up: %v", err)
	}

	// 2회차: 이제 두 파일 모두 pylon이 쓴 것이므로 백업이 갱신되면 안 된다.
	if _, _, err := ensureRootAgentFiles(root, nil); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(agentsBak)
	if string(second) != string(first) {
		t.Errorf("pylon-authored AGENTS.md must not be re-backed-up: %.60q", second)
	}
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

// 반대로, 저작되지 않은 부트스트랩 스텁은 버전이 올라도 백업 쓰레기를 남기지 않는다.
func TestEnsureRootAgentFilesDoesNotBackUpStaleBootstrapStub(t *testing.T) {
	root := t.TempDir()
	stub := strings.Replace(buildBootstrapAgentsMD(root, nil),
		"pylon-usage-version: 1", "pylon-usage-version: 0", 1)
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte(stub), 0644); err != nil {
		t.Fatal(err)
	}
	_, backedUp, err := ensureRootAgentFiles(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(backedUp) != 0 {
		t.Errorf("an unauthored bootstrap stub should not be backed up, got %v", backedUp)
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
	authored := "<!-- pylon-usage-version: 1 -->\n# 사용자 세션이 저작한 가이드"
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
