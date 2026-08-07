package cli

import (
	"fmt"
	"os"
	"path/filepath"
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

// rootFileBackupSuffix is appended to a hand-written root file that pylon is about to
// replace, so `pylon init` in a repo that already has its own CLAUDE.md/AGENTS.md never
// destroys it silently — the root files are gitignored, so an untracked original would
// otherwise be unrecoverable.
const rootFileBackupSuffix = ".pylon-bak"

// backupIfHandWritten renames path aside when it exists and was not written by pylon.
// pylonAuthored decides that from the current content; a file pylon itself wrote is
// replaced in place, so repeated launches never churn out backups. Reports whether a
// backup was taken.
func backupIfHandWritten(path string, pylonAuthored func([]byte) bool) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, nil // 없으면 백업할 것도 없다
	}
	if pylonAuthored(data) {
		return false, nil
	}
	if err := os.Rename(path, path+rootFileBackupSuffix); err != nil {
		return false, fmt.Errorf("%s 백업 실패: %w", filepath.Base(path), err)
	}
	return true, nil
}

// ensureRootAgentFiles keeps the two workspace-root files in the desired state:
// CLAUDE.md is always (re)written to the deterministic @AGENTS.md import marker, and
// AGENTS.md is (re)written to the bootstrap ONLY when stale — a session-authored,
// current AGENTS.md is left untouched. A pre-existing hand-written file is moved to
// <name>.pylon-bak first. Returns whether AGENTS.md was bootstrapped, and the names of
// any files backed up so callers can tell the user.
func ensureRootAgentFiles(root string, projects []config.ProjectInfo) (bool, []string, error) {
	var backedUp []string

	claudePath := layout.RootClaudePath(root)
	// pylon이 쓴 CLAUDE.md는 마커 한 줄뿐이다 — 그 외 내용은 사용자 것이다.
	ok, err := backupIfHandWritten(claudePath, func(b []byte) bool {
		return strings.TrimSpace(string(b)) == strings.TrimSpace(buildClaudeMDPointer())
	})
	if err != nil {
		return false, backedUp, err
	}
	if ok {
		backedUp = append(backedUp, filepath.Base(claudePath))
	}
	if err := os.WriteFile(claudePath, []byte(buildClaudeMDPointer()), 0644); err != nil {
		return false, backedUp, fmt.Errorf("CLAUDE.md 생성 실패: %w", err)
	}

	if !agentsMDStale(root) {
		return false, backedUp, nil
	}

	agentsPath := layout.RootAgentsPath(root)
	// 스탬프가 있으면 pylon/세션이 저작한 것이다. 스탬프 없는 파일만 사용자 것으로 본다.
	ok, err = backupIfHandWritten(agentsPath, func(b []byte) bool {
		return usageVersionRe.Find(b) != nil
	})
	if err != nil {
		return false, backedUp, err
	}
	if ok {
		backedUp = append(backedUp, filepath.Base(agentsPath))
	}
	if err := os.WriteFile(agentsPath, []byte(buildBootstrapAgentsMD(root, projects)), 0644); err != nil {
		return false, backedUp, fmt.Errorf("AGENTS.md 부트스트랩 실패: %w", err)
	}
	return true, backedUp, nil
}
