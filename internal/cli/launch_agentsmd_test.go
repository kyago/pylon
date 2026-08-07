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
	bootstrapped, err := ensureRootAgentFiles(root, nil)
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

func TestEnsureRootAgentFilesPreservesFreshAgentsMD(t *testing.T) {
	root := t.TempDir()
	authored := "<!-- pylon-usage-version: 1 -->\n# 사용자 세션이 저작한 가이드"
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte(authored), 0644); err != nil {
		t.Fatal(err)
	}
	bootstrapped, err := ensureRootAgentFiles(root, nil)
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
