package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
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

func TestBuildAgentsBlockEmbedsManualAndFacts(t *testing.T) {
	root := t.TempDir()
	out := buildAgentsBlock(root, []config.ProjectInfo{{Name: "api"}})
	if !strings.HasPrefix(out, agentsBlockBegin+"\n") || !strings.HasSuffix(out, agentsBlockEnd+"\n") {
		t.Errorf("block must be wrapped in markers: %s", out)
	}
	manual, err := embeddedReference.ReadFile("reference/pylon-usage.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, strings.TrimSpace(string(manual))) {
		t.Error("block must carry the embedded manual verbatim")
	}
	if !strings.Contains(out, "`"+root+"`") || !strings.Contains(out, "api") {
		t.Errorf("block should list workspace facts: %s", out)
	}
	// 마이그레이션 인식자와 겹치면 새 블록이 레거시 저작 가이드로 오인되어 매번 백업된다.
	if isAuthoredLegacyBlock([]byte(out)) || strings.Contains(out, bootstrapAgentsMDHeading) {
		t.Error("block must not match the legacy recognizers")
	}
	// 매뉴얼 본문의 마커 예시가 블록 경계로 파싱되면 안 된다.
	if s, e, state := findAgentsBlock([]byte(out)); state != agentsBlockComplete || s != 0 || e != len(out) {
		t.Errorf("block must parse as one complete block: state=%v [%d,%d) of %d", state, s, e, len(out))
	}
	if buildAgentsBlock(root, []config.ProjectInfo{{Name: "api"}}) != out {
		t.Error("block must be deterministic")
	}
}

func TestEnsureRootAgentFilesWritesPointerAndBootstrapWhenMissing(t *testing.T) {
	root := t.TempDir()
	updated, _, err := ensureRootAgentFiles(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !updated {
		t.Error("missing AGENTS.md should be updated")
	}
	ptr, _ := os.ReadFile(layout.RootClaudePath(root))
	if strings.TrimSpace(string(ptr)) != "@AGENTS.md" {
		t.Errorf("CLAUDE.md should be the import marker, got %q", ptr)
	}
	if _, err := os.Stat(layout.RootAgentsPath(root)); err != nil {
		t.Errorf("AGENTS.md bootstrap not written: %v", err)
	}
}

func TestEnsureRootAgentFilesPrependsBlockToUserAgentsMD(t *testing.T) {
	root := t.TempDir()
	handAgents := "# 우리 팀 가이드\n스탬프 없는 수작업 파일"
	handClaude := "# 우리 팀 CLAUDE.md\n마커가 아닌 수작업 파일"
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte(handAgents), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(layout.RootClaudePath(root), []byte(handClaude), 0644); err != nil {
		t.Fatal(err)
	}

	updated, backedUp, err := ensureRootAgentFiles(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !updated {
		t.Error("user AGENTS.md without a block should get the block prepended")
	}

	// 사용자 AGENTS.md는 백업이 아니라 그 자리에 보존되고, 블록이 앞에 붙는다.
	nowAgents, _ := os.ReadFile(layout.RootAgentsPath(root))
	if !strings.HasSuffix(string(nowAgents), handAgents) {
		t.Errorf("user AGENTS.md content must be preserved in place: %q", nowAgents)
	}
	if !strings.Contains(string(nowAgents), agentsBlockBegin) || !strings.Contains(string(nowAgents), agentsBlockEnd) {
		t.Errorf("pylon block not prepended: %q", nowAgents)
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

func TestEnsureRootAgentFilesKeepsManagedBlockWithinCodexDefaultLimit(t *testing.T) {
	root := t.TempDir()
	original := strings.Repeat("사용자 규칙 ", 4000)
	if len(original) <= 32*1024 {
		t.Fatal("test fixture must exceed Codex's default project instruction limit")
	}
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := ensureRootAgentFiles(root, nil); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(layout.RootAgentsPath(root))
	if err != nil {
		t.Fatal(err)
	}
	end := bytes.Index(got, []byte(agentsBlockEnd))
	if end < 0 || end >= 32*1024 {
		t.Fatalf("managed block ends outside Codex's default 32KiB limit: %d", end)
	}
	if !bytes.HasSuffix(got, []byte(original)) {
		t.Fatal("prepending the managed block changed user content")
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
	updated, backedUp, err := ensureRootAgentFiles(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if updated {
		t.Error("second run must not rewrite a current block")
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

// 블록 도입 이전에 세션이 저작한 전체 파일 가이드는 교체되지만 버려서는 안 된다.
func TestEnsureRootAgentFilesBacksUpStaleAuthoredAgentsMD(t *testing.T) {
	root := t.TempDir()
	authored := "<!-- pylon-usage-version: 0 -->\n# 세션이 저작한 소중한 가이드\n상세 운영 규칙"
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte(authored), 0644); err != nil {
		t.Fatal(err)
	}
	updated, backedUp, err := ensureRootAgentFiles(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !updated {
		t.Error("legacy AGENTS.md should be replaced")
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

// 내용이 달라진 블록(프로젝트 목록·매뉴얼 변경)은 블록만 교체된다 — 블록 밖 사용자 내용은
// 백업 없이 그 자리에 보존.
func TestEnsureRootAgentFilesReplacesOutdatedBlockPreservingUserContent(t *testing.T) {
	root := t.TempDir()
	userTop := "# 우리 팀 규칙\n블록 밖 내용\n\n"
	userBottom := "\n# 블록 아래 추가 규칙\n"
	outdated := buildAgentsBlock(root, []config.ProjectInfo{{Name: "removed-project"}})
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte(userTop+outdated+userBottom), 0600); err != nil {
		t.Fatal(err)
	}
	updated, backedUp, err := ensureRootAgentFiles(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !updated {
		t.Error("outdated block should be rewritten")
	}
	if len(backedUp) != 0 {
		t.Errorf("pylon-generated block replacement must not back anything up, got %v", backedUp)
	}
	got, _ := os.ReadFile(layout.RootAgentsPath(root))
	if string(got) != userTop+buildAgentsBlock(root, nil)+userBottom {
		t.Errorf("block not replaced in place: %q", got)
	}
	info, err := os.Stat(layout.RootAgentsPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("AGENTS.md mode = %o, want 600", info.Mode().Perm())
	}
}

func TestEnsureRootAgentFilesRejectsMalformedBlockWithoutChangingUserContent(t *testing.T) {
	blockBody := "\n<!-- pylon-usage-version: 0 -->\n" + bootstrapAgentsMDHeading + "\n"
	tests := map[string]string{
		"missing end":   "# 우리 팀 규칙\n\n" + agentsBlockBegin + blockBody,
		"missing begin": "# 우리 팀 규칙\n\n" + blockBody + agentsBlockEnd + "\n",
		"duplicate": agentsBlockBegin + blockBody + agentsBlockEnd + "\n" +
			agentsBlockBegin + blockBody + agentsBlockEnd + "\n",
	}
	for name, original := range tests {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			handClaude := "# 우리 팀 CLAUDE.md\n수작업 파일"
			if err := os.WriteFile(layout.RootAgentsPath(root), []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(layout.RootClaudePath(root), []byte(handClaude), 0644); err != nil {
				t.Fatal(err)
			}

			if _, _, err := ensureRootAgentFiles(root, nil); !errors.Is(err, errAgentsBlockMalformed) {
				t.Fatalf("malformed pylon block must stop reconciliation, got %v", err)
			}
			got, err := os.ReadFile(layout.RootAgentsPath(root))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != original {
				t.Fatalf("malformed block changed user content:\ngot:  %q\nwant: %q", got, original)
			}
			// malformed 중단은 CLAUDE.md를 백업·교체하기 전에 일어나야 한다 — 에러 경로에서는
			// backedUp이 호출부에서 버려져 백업 사실이 통보되지 않기 때문이다.
			gotClaude, err := os.ReadFile(layout.RootClaudePath(root))
			if err != nil || string(gotClaude) != handClaude {
				t.Fatalf("CLAUDE.md must be untouched on malformed abort: %q, err=%v", gotClaude, err)
			}
			if _, err := os.Stat(layout.RootClaudePath(root) + rootFileBackupSuffix); !os.IsNotExist(err) {
				t.Fatal("CLAUDE.md must not be backed up on malformed abort")
			}
		})
	}
}

// 마커에 들여쓰기·꼬리 공백이 붙어도(포매터, 세션 재저작) 블록으로 인정된다 — absent로
// 강등되어 레거시 경로가 사용자 내용을 백업 없이 덮어쓰는 일이 없어야 한다.
func TestEnsureRootAgentFilesToleratesWhitespaceAroundMarkers(t *testing.T) {
	root := t.TempDir()
	userTop := "# 우리 팀 규칙\n\n"
	mangled := userTop +
		"  " + agentsBlockBegin + " \n" +
		"<!-- pylon-usage-version: 0 -->\n" + bootstrapAgentsMDHeading + "\n" +
		"\t" + agentsBlockEnd + "\n"
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte(mangled), 0644); err != nil {
		t.Fatal(err)
	}
	updated, backedUp, err := ensureRootAgentFiles(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !updated {
		t.Error("whitespace-mangled legacy stub block should still be replaced")
	}
	if len(backedUp) != 0 {
		t.Errorf("block replacement must not back anything up, got %v", backedUp)
	}
	got, _ := os.ReadFile(layout.RootAgentsPath(root))
	if !strings.HasPrefix(string(got), userTop) {
		t.Errorf("user content above the block was destroyed: %q", got)
	}
	if string(got) != userTop+buildAgentsBlock(root, nil) {
		t.Errorf("block not refreshed: %q", got)
	}
}

func TestEnsureRootAgentFilesTreatsInlineMarkerExamplesAsUserContent(t *testing.T) {
	root := t.TempDir()
	original := "# 우리 팀 규칙\n마커 예시: `" + agentsBlockBegin + "` ~ `" + agentsBlockEnd + "`\n"
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := ensureRootAgentFiles(root, nil); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(layout.RootAgentsPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(got), original) {
		t.Fatalf("inline marker example must remain user content: %q", got)
	}
}

func TestEnsureRootAgentFilesTreatsNonLeadingVersionStampAsUserContent(t *testing.T) {
	root := t.TempDir()
	original := "# 우리 팀 규칙\n\n문서에 쓰는 스탬프 예시:\n<!-- pylon-usage-version: 1 -->\n"
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	_, backedUp, err := ensureRootAgentFiles(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(backedUp) != 0 {
		t.Fatalf("a non-leading stamp example must not trigger legacy migration: %v", backedUp)
	}
	got, err := os.ReadFile(layout.RootAgentsPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(got), original) {
		t.Fatalf("version stamp example must remain user content: %q", got)
	}
}

func TestEnsureRootAgentFilesRejectsUnreadableAgentsMDBeforeChangingClaudeMD(t *testing.T) {
	root := t.TempDir()
	agentsPath := layout.RootAgentsPath(root)
	claudePath := layout.RootClaudePath(root)
	originalAgents := []byte("# 읽을 수 없는 사용자 규칙\n")
	originalClaude := []byte("# 사용자 CLAUDE.md\n")
	if err := os.WriteFile(agentsPath, originalAgents, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(agentsPath, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(agentsPath, 0600) })
	if _, err := os.ReadFile(agentsPath); err == nil {
		t.Skip("filesystem does not enforce unreadable file mode")
	}
	if err := os.WriteFile(claudePath, originalClaude, 0644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := ensureRootAgentFiles(root, nil); err == nil {
		t.Fatal("an unreadable AGENTS.md must abort reconciliation")
	}
	gotClaude, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotClaude) != string(originalClaude) {
		t.Fatalf("CLAUDE.md changed before AGENTS.md read failure: %q", gotClaude)
	}
	if _, err := os.Stat(claudePath + rootFileBackupSuffix); !os.IsNotExist(err) {
		t.Fatal("CLAUDE.md must not be backed up before AGENTS.md preflight succeeds")
	}
}

func TestEnsureRootAgentFilesDoesNotReplaceUnreadableClaudeMD(t *testing.T) {
	root := t.TempDir()
	agentsPath := layout.RootAgentsPath(root)
	claudePath := layout.RootClaudePath(root)
	if err := os.WriteFile(agentsPath, []byte(buildAgentsBlock(root, nil)), 0644); err != nil {
		t.Fatal(err)
	}
	original := []byte("# 읽을 수 없는 사용자 CLAUDE.md\n")
	if err := os.WriteFile(claudePath, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(claudePath, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(claudePath, 0600) })
	if _, err := os.ReadFile(claudePath); err == nil {
		t.Skip("filesystem does not enforce unreadable file mode")
	}

	if _, _, err := ensureRootAgentFiles(root, nil); err == nil {
		t.Fatal("an unreadable CLAUDE.md must abort reconciliation")
	}
	if err := os.Chmod(claudePath, 0600); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Fatalf("unreadable CLAUDE.md was replaced: %q", got)
	}
}

func TestWriteAgentsFileDoesNotReplaceDanglingSymlink(t *testing.T) {
	root := t.TempDir()
	path := layout.RootAgentsPath(root)
	missingTarget := filepath.Join(root, "missing", "AGENTS.md")
	if err := os.Symlink(missingTarget, path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if err := writeAgentsFile(path, []byte("replacement\n")); err == nil {
		t.Fatal("writing through a dangling symlink must fail safely")
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("dangling symlink was replaced by a regular file")
	}
}

func TestEnsureRootAgentFilesRejectsDanglingAgentsMDBeforeChangingClaudeMD(t *testing.T) {
	root := t.TempDir()
	agentsPath := layout.RootAgentsPath(root)
	claudePath := layout.RootClaudePath(root)
	if err := os.Symlink(filepath.Join(root, "missing-AGENTS.md"), agentsPath); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	originalClaude := []byte("# 사용자 CLAUDE.md\n")
	if err := os.WriteFile(claudePath, originalClaude, 0644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := ensureRootAgentFiles(root, nil); err == nil {
		t.Fatal("a dangling AGENTS.md symlink must abort reconciliation")
	}
	got, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(originalClaude) {
		t.Fatalf("CLAUDE.md changed before dangling AGENTS.md preflight failed: %q", got)
	}
}

func TestEnsureRootAgentFilesRecognizesLegacyBootstrapWithoutStamp(t *testing.T) {
	root := t.TempDir()
	legacy := bootstrapAgentsMDHeading + "\n\n이 파일은 아직 저작되지 않았습니다.\n"
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}

	_, backedUp, err := ensureRootAgentFiles(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(backedUp) != 0 {
		t.Fatalf("legacy bootstrap stub must be replaced without backup: %v", backedUp)
	}
	got, err := os.ReadFile(layout.RootAgentsPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(got), agentsBlockBegin+"\n") {
		t.Fatalf("legacy bootstrap stub was retained as user content: %q", got)
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

func TestEnsureRootAgentFilesBacksUpClaudeMDWithLegacyHeadingOnly(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte(buildAgentsBlock(root, nil)), 0644); err != nil {
		t.Fatal(err)
	}
	original := legacyClaudeMDHeading + "\n\n우리 팀이 직접 작성한 규칙\n"
	claudePath := layout.RootClaudePath(root)
	if err := os.WriteFile(claudePath, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	_, backedUp, err := ensureRootAgentFiles(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(backedUp) != 1 || backedUp[0] != "CLAUDE.md" {
		t.Fatalf("title-only user CLAUDE.md must be backed up: %v", backedUp)
	}
	got, err := os.ReadFile(claudePath + rootFileBackupSuffix)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Fatalf("CLAUDE.md backup changed user content: %q", got)
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

func TestEnsureRootAgentFilesLeavesCurrentBlockUntouched(t *testing.T) {
	root := t.TempDir()
	current := "# 사용자 규칙\n\n" + buildAgentsBlock(root, nil) + "\n# 아래 규칙\n"
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte(current), 0644); err != nil {
		t.Fatal(err)
	}
	updated, _, err := ensureRootAgentFiles(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if updated {
		t.Error("a current block must not be rewritten")
	}
	if got, _ := os.ReadFile(layout.RootAgentsPath(root)); string(got) != current {
		t.Errorf("current AGENTS.md was overwritten: %q", got)
	}
}

// 블록 안은 pylon 소유다 — 직접 고친 내용은 백업 없이 되돌아간다.
func TestEnsureRootAgentFilesRevertsEditsInsideBlock(t *testing.T) {
	root := t.TempDir()
	edited := strings.Replace(buildAgentsBlock(root, nil), "## 이 워크스페이스", "## 내가 고친 제목", 1)
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte(edited), 0644); err != nil {
		t.Fatal(err)
	}
	_, backedUp, err := ensureRootAgentFiles(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(backedUp) != 0 {
		t.Errorf("edits inside the generated block are not backed up, got %v", backedUp)
	}
	if got, _ := os.ReadFile(layout.RootAgentsPath(root)); string(got) != buildAgentsBlock(root, nil) {
		t.Errorf("edit inside the block was not reverted: %q", got)
	}
}

// 이전 방식에서 세션이 저작한 블록(스탬프 보유, 스텁 아님)은 .pylon-bak으로 한 번 백업한 뒤
// 생성 블록으로 교체한다. 블록 밖 내용은 그대로.
func TestEnsureRootAgentFilesBacksUpAuthoredLegacyBlockOnce(t *testing.T) {
	root := t.TempDir()
	userTop := "# 우리 팀 규칙\n블록 밖 내용\n\n"
	userBottom := "\n# 블록 아래 추가 규칙\n"
	authored := agentsBlockBegin + "\n<!-- pylon-usage-version: 5 -->\n# 세션이 저작한 운영 가이드\n\n- 규칙 1\n" + agentsBlockEnd + "\n"
	original := userTop + authored + userBottom
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	updated, backedUp, err := ensureRootAgentFiles(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !updated || len(backedUp) != 1 || backedUp[0] != "AGENTS.md" {
		t.Fatalf("authored legacy block must be backed up and replaced: updated=%v backedUp=%v", updated, backedUp)
	}
	if bak, _ := os.ReadFile(layout.RootAgentsPath(root) + rootFileBackupSuffix); string(bak) != original {
		t.Errorf("backup must hold the original bytes: %q", bak)
	}
	if got, _ := os.ReadFile(layout.RootAgentsPath(root)); string(got) != userTop+buildAgentsBlock(root, nil)+userBottom {
		t.Errorf("block not replaced in place: %q", got)
	}
	// 두 번째 실행은 아무것도 하지 않는다 — 백업이 늘어나면 안 된다.
	updated, backedUp, err = ensureRootAgentFiles(root, nil)
	if err != nil || updated || len(backedUp) != 0 {
		t.Errorf("second run must be a no-op: updated=%v backedUp=%v err=%v", updated, backedUp, err)
	}
}
