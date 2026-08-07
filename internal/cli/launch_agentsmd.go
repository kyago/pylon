package cli

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/layout"
)

// pylonUsageVersion is the version of the embedded pylon usage manual
// (internal/cli/reference/pylon-usage.md). Bump it ONLY when that manual changes —
// it is the sole trigger that marks an existing AGENTS.md stale and forces the next
// session to re-author it. Independent of pylon's CalVer release version.
const pylonUsageVersion = 1

var usageVersionRe = regexp.MustCompile(`pylon-usage-version:\s*(\d+)`)

// buildClaudeMDPointer returns the workspace-root CLAUDE.md content: a single
// Claude Code import marker that pulls AGENTS.md into the session prompt. Kept
// deterministic so launch may rewrite it every time without churn.
func buildClaudeMDPointer() string {
	return "@AGENTS.md\n"
}

// buildBootstrapAgentsMD returns the short placeholder AGENTS.md written when the
// file is missing or stale. Its job is to make the launched session author the real
// guide on its first turn; it also stands on its own as a minimal fallback if
// authoring never happens.
func buildBootstrapAgentsMD(root string, projects []config.ProjectInfo) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<!-- pylon-usage-version: %d -->\n", pylonUsageVersion)
	b.WriteString("# pylon 워크스페이스 — 운영 가이드 (자동 생성 부트스트랩)\n\n")
	b.WriteString("이 파일은 아직 이 워크스페이스에 맞게 저작되지 않았습니다.\n")
	b.WriteString("**다른 작업을 하기 전에**, `.pylon/reference/pylon-usage.md`와 실제 리포지토리·`.pylon/`을 읽고\n")
	b.WriteString("이 파일(AGENTS.md)을 이 워크스페이스의 운영 가이드로 다시 작성하세요.\n")
	fmt.Fprintf(&b, "완료하면 최상단 스탬프를 `<!-- pylon-usage-version: %d -->`로 남기세요.\n\n", pylonUsageVersion)
	fmt.Fprintf(&b, "- **워크스페이스 루트**: `%s`\n", root)
	b.WriteString("- **매뉴얼**: `.pylon/reference/pylon-usage.md`\n")
	b.WriteString("- **설정**: `.pylon/config.yml` / **도메인 지식**: `.pylon/domain/` / **에이전트**: `.pylon/agents/`\n")
	if len(projects) > 0 {
		fmt.Fprintf(&b, "- **프로젝트 %d개**: ", len(projects))
		names := make([]string, len(projects))
		for i, p := range projects {
			names[i] = p.Name
		}
		b.WriteString(strings.Join(names, ", "))
		b.WriteString("\n")
	} else {
		b.WriteString("- **프로젝트**: 없음 — `pylon add-project <git-url>`로 추가\n")
	}
	return b.String()
}

// agentsMDStale reports whether the workspace-root AGENTS.md needs (re)authoring:
// missing, no parseable version stamp, or a stamp older than pylonUsageVersion.
func agentsMDStale(root string) bool {
	data, err := os.ReadFile(layout.RootAgentsPath(root))
	if err != nil {
		return true
	}
	m := usageVersionRe.FindSubmatch(data)
	if m == nil {
		return true
	}
	v, err := strconv.Atoi(string(m[1]))
	if err != nil {
		return true
	}
	return v < pylonUsageVersion
}

// ensureRootAgentFiles keeps the two workspace-root files in the desired state:
// CLAUDE.md is always (re)written to the deterministic @AGENTS.md import marker, and
// AGENTS.md is (re)written to the bootstrap ONLY when stale — a session-authored,
// current AGENTS.md is left untouched. Returns whether AGENTS.md was bootstrapped.
func ensureRootAgentFiles(root string, projects []config.ProjectInfo) (bool, error) {
	if err := os.WriteFile(layout.RootClaudePath(root), []byte(buildClaudeMDPointer()), 0644); err != nil {
		return false, fmt.Errorf("CLAUDE.md 생성 실패: %w", err)
	}
	if !agentsMDStale(root) {
		return false, nil
	}
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte(buildBootstrapAgentsMD(root, projects)), 0644); err != nil {
		return false, fmt.Errorf("AGENTS.md 부트스트랩 실패: %w", err)
	}
	return true, nil
}
