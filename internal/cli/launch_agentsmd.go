package cli

import (
	"bytes"
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
const pylonUsageVersion = 4

var usageVersionRe = regexp.MustCompile(`pylon-usage-version:\s*(\d+)`)

// agentsBlockBegin/agentsBlockEnd delimit the pylon-managed region inside the
// workspace-root AGENTS.md. pylon and the launched session only ever rewrite what
// lies between the markers; anything a user writes outside the block is preserved
// verbatim across launch, doctor, and version bumps.
const agentsBlockBegin = "<!-- pylon:begin -->"
const agentsBlockEnd = "<!-- pylon:end -->"

// buildClaudeMDPointer returns the workspace-root CLAUDE.md content: a single
// Claude Code import marker that pulls AGENTS.md into the session prompt. Kept
// deterministic so launch may rewrite it every time without churn.
func buildClaudeMDPointer() string {
	return "@AGENTS.md\n"
}

// buildAgentsBlock returns the pylon-managed block (markers included) written into
// AGENTS.md when the block is missing or stale. Its job is to make the launched
// session author the real guide inside the block on its first turn; it also stands
// on its own as a minimal fallback if authoring never happens.
// bootstrapAgentsMDHeading is the heading buildAgentsBlock emits. It is the
// signature that tells an unauthored stub apart from a guide a session actually wrote:
// only the stub may be discarded without a backup (pre-block legacy files only —
// with the block in place, replacement never touches user content).
const bootstrapAgentsMDHeading = "# pylon 워크스페이스 — 운영 가이드 (자동 생성 부트스트랩)"

// legacyClaudeMDHeading is the first line the pre-AGENTS.md generator (buildRootCLAUDEMD,
// removed in this change) wrote into workspace CLAUDE.md. Kept as a migration recognizer:
// every workspace created before this change has such a file, and it is pylon's own
// output, so it is replaced by the marker without a backup. Do not delete as unused.
const legacyClaudeMDHeading = "# Pylon — AI 멀티도메인 워크스페이스"

func buildAgentsBlock(root string, projects []config.ProjectInfo) string {
	var b strings.Builder
	b.WriteString(agentsBlockBegin + "\n")
	fmt.Fprintf(&b, "<!-- pylon-usage-version: %d -->\n", pylonUsageVersion)
	b.WriteString(bootstrapAgentsMDHeading + "\n\n")
	b.WriteString("이 블록(`pylon:begin`~`pylon:end`)은 pylon이 관리합니다 — 블록 밖에 적은 내용은 절대 건드리지 않으니\n")
	b.WriteString("워크스페이스 자체 규칙은 블록 밖에 자유롭게 작성하세요.\n\n")
	b.WriteString("이 블록은 아직 이 워크스페이스에 맞게 저작되지 않았습니다.\n")
	b.WriteString("**다른 작업을 하기 전에**, `.pylon/reference/pylon-usage.md`와 실제 리포지토리·`.pylon/`을 읽고\n")
	b.WriteString("이 블록의 내용을 이 워크스페이스의 운영 가이드로 다시 작성하세요.\n")
	fmt.Fprintf(&b, "마커 두 줄과 `<!-- pylon-usage-version: %d -->` 스탬프는 블록 안에 그대로 유지하고,\n", pylonUsageVersion)
	b.WriteString("블록 밖의 내용은 수정하지 마세요.\n\n")
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
	b.WriteString(agentsBlockEnd + "\n")
	return b.String()
}

// findAgentsBlock returns the [start,end) byte range of the pylon block including
// both markers (and the newline right after the end marker, so replacement is
// idempotent), or ok=false when the file has no complete block.
func findAgentsBlock(data []byte) (start, end int, ok bool) {
	s := bytes.Index(data, []byte(agentsBlockBegin))
	if s < 0 {
		return 0, 0, false
	}
	rel := bytes.Index(data[s:], []byte(agentsBlockEnd))
	if rel < 0 {
		return 0, 0, false
	}
	e := s + rel + len(agentsBlockEnd)
	if e < len(data) && data[e] == '\n' {
		e++
	}
	return s, e, true
}

// stripAgentsBlock rewrites path with the pylon block removed, leaving user
// content untouched. A file without a complete block is left as-is.
func stripAgentsBlock(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	s, e, ok := findAgentsBlock(data)
	if !ok {
		return nil
	}
	out := append(append([]byte{}, data[:s]...), data[e:]...)
	return os.WriteFile(path, out, 0644)
}

// agentsMDStale reports whether the pylon block in the workspace-root AGENTS.md
// needs (re)authoring: file missing, no complete block, no parseable version stamp
// inside the block, or a stamp older than pylonUsageVersion. The stamp is only
// looked for inside the block, so user content cannot mask a stale block.
func agentsMDStale(root string) bool {
	data, err := os.ReadFile(layout.RootAgentsPath(root))
	if err != nil {
		return true
	}
	s, e, ok := findAgentsBlock(data)
	if !ok {
		return true
	}
	m := usageVersionRe.FindSubmatch(data[s:e])
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

// maxRootFileBackups bounds the .pylon-bak / .pylon-bak.1 / ... search so a pathological
// workspace cannot spin forever instead of reporting a problem.
const maxRootFileBackups = 100

// backupIfHandWritten renames path aside when it exists and was not written by pylon.
// pylonAuthored decides that from the current content; a file pylon itself produced is
// replaced in place, so ordinary launches never churn out backups. An existing backup is
// never clobbered — the next free suffix is used. Reports whether a backup was taken.
func backupIfHandWritten(path string, pylonAuthored func([]byte) bool) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, nil // 없으면 백업할 것도 없다
	}
	if pylonAuthored(data) {
		return false, nil
	}
	dest := path + rootFileBackupSuffix
	for i := 1; ; i++ {
		if _, err := os.Lstat(dest); os.IsNotExist(err) {
			break
		}
		if i > maxRootFileBackups {
			return false, fmt.Errorf("%s 백업 실패: 백업 파일이 너무 많습니다 (%s*)",
				filepath.Base(path), filepath.Base(path)+rootFileBackupSuffix)
		}
		dest = fmt.Sprintf("%s%s.%d", path, rootFileBackupSuffix, i)
	}
	if err := os.Rename(path, dest); err != nil {
		return false, fmt.Errorf("%s 백업 실패: %w", filepath.Base(path), err)
	}
	return true, nil
}

// ensureRootAgentFiles keeps the two workspace-root files in the desired state:
// CLAUDE.md is always (re)written to the deterministic @AGENTS.md import marker, and
// the pylon block inside AGENTS.md is re-bootstrapped ONLY when stale — a
// session-authored, current block is left untouched, and content outside the block
// is NEVER modified. A user's own AGENTS.md (no block) keeps its content and gets
// the block appended; only pre-block legacy pylon files are replaced whole (with a
// .pylon-bak backup when session-authored). Returns whether the block was
// bootstrapped, and the names of any files backed up so callers can tell the user.
func ensureRootAgentFiles(root string, projects []config.ProjectInfo) (bool, []string, error) {
	var backedUp []string

	claudePath := layout.RootClaudePath(root)
	// pylon이 쓴 CLAUDE.md는 두 가지뿐이다: 현재의 마커, 그리고 이 변경 이전 워크스페이스에
	// 남아 있는 하드코딩 프롬프트. 둘 다 pylon 산출물이므로 백업 없이 교체한다.
	ok, err := backupIfHandWritten(claudePath, func(b []byte) bool {
		s := strings.TrimSpace(string(b))
		return s == strings.TrimSpace(buildClaudeMDPointer()) ||
			strings.HasPrefix(s, legacyClaudeMDHeading)
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
	block := buildAgentsBlock(root, projects)
	data, readErr := os.ReadFile(agentsPath)
	if readErr != nil {
		// 파일 없음 → 블록만으로 새로 만든다.
		if err := os.WriteFile(agentsPath, []byte(block), 0644); err != nil {
			return false, backedUp, fmt.Errorf("AGENTS.md 부트스트랩 실패: %w", err)
		}
		return true, backedUp, nil
	}

	if s, e, blockFound := findAgentsBlock(data); blockFound {
		// stale 블록만 교체 — 블록 밖 사용자 내용은 그대로 둔다. 블록 안은 pylon 소유이고
		// 다음 세션이 팩트로부터 재저작하므로 백업하지 않는다.
		out := string(data[:s]) + block + string(data[e:])
		if err := os.WriteFile(agentsPath, []byte(out), 0644); err != nil {
			return false, backedUp, fmt.Errorf("AGENTS.md 부트스트랩 실패: %w", err)
		}
		return true, backedUp, nil
	}

	if usageVersionRe.Match(data) || strings.Contains(string(data), bootstrapAgentsMDHeading) {
		// 블록 도입 이전의 pylon 전체 파일. 저작되지 않은 스텁만 그냥 버린다. 스탬프 유무로
		// 판단하면 안 된다 — 스탬프를 가진 세션 저작 가이드까지 조용히 사라진다.
		ok, err = backupIfHandWritten(agentsPath, func(b []byte) bool {
			return strings.Contains(string(b), bootstrapAgentsMDHeading)
		})
		if err != nil {
			return false, backedUp, err
		}
		if ok {
			backedUp = append(backedUp, filepath.Base(agentsPath))
		}
		if err := os.WriteFile(agentsPath, []byte(block), 0644); err != nil {
			return false, backedUp, fmt.Errorf("AGENTS.md 부트스트랩 실패: %w", err)
		}
		return true, backedUp, nil
	}

	// 사용자가 직접 쓴 AGENTS.md — 내용을 보존하고 블록을 끝에 덧붙인다.
	out := string(data)
	if out != "" && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	if out != "" {
		out += "\n"
	}
	out += block
	if err := os.WriteFile(agentsPath, []byte(out), 0644); err != nil {
		return false, backedUp, fmt.Errorf("AGENTS.md 부트스트랩 실패: %w", err)
	}
	return true, backedUp, nil
}
