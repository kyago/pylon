package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/fsutil"
	"github.com/kyago/pylon/internal/layout"
)

// pylonUsageVersion is the version of the embedded pylon usage manual
// (internal/cli/reference/pylon-usage.md). Bump it ONLY when that manual changes —
// it is the sole trigger that marks an existing AGENTS.md stale and forces the next
// session to re-author it. Independent of pylon's CalVer release version.
const pylonUsageVersion = 5

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
	b.WriteString("관리 블록에는 `.pylon/`과 등록 프로젝트에서 확인한 기능만 반영하고,\n")
	b.WriteString("pylon이 제공하지 않는 외부 도구·스킬을 추가하거나 필수 절차로 지정하지 마세요.\n\n")
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

type agentsBlockState uint8

const (
	agentsBlockAbsent agentsBlockState = iota
	agentsBlockComplete
	agentsBlockMalformed
)

// findAgentsBlock returns the [start,end) byte range of the single complete
// pylon block, including both marker lines and the end marker's newline. Markers
// count only when they occupy an entire line (surrounding whitespace tolerated, so
// an indented or trailing-space marker still parses); inline examples with other
// text on the line are user content.
// Missing, reversed, or duplicate marker lines are reported as malformed so
// callers never mistake a damaged managed block for a legacy whole-file guide.
func findAgentsBlock(data []byte) (start, end int, state agentsBlockState) {
	var begins, ends []int
	var endLineEnds []int
	for offset := 0; offset < len(data); {
		rel := bytes.IndexByte(data[offset:], '\n')
		lineEnd := len(data)
		next := len(data)
		if rel >= 0 {
			lineEnd = offset + rel
			next = lineEnd + 1
		}
		line := bytes.TrimSpace(data[offset:lineEnd])
		switch string(line) {
		case agentsBlockBegin:
			begins = append(begins, offset)
		case agentsBlockEnd:
			ends = append(ends, offset)
			endLineEnds = append(endLineEnds, next)
		}
		offset = next
	}

	if len(begins) == 0 && len(ends) == 0 {
		return 0, 0, agentsBlockAbsent
	}
	if len(begins) != 1 || len(ends) != 1 || begins[0] >= ends[0] {
		return 0, 0, agentsBlockMalformed
	}
	return begins[0], endLineEnds[0], agentsBlockComplete
}

// errAgentsBlockMalformed is the shared sentinel for a damaged marker pair, so
// callers can errors.Is it apart from I/O failures.
var errAgentsBlockMalformed = errors.New("pylon 관리 블록 마커가 불완전하거나 중복되었습니다")

func agentsBlockMalformedError(path string) error {
	return fmt.Errorf("%s: %w — `%s`와 `%s`가 각각 정확히 한 줄씩, begin→end 순서로 있도록 수정한 뒤 다시 실행하세요",
		path, errAgentsBlockMalformed, agentsBlockBegin, agentsBlockEnd)
}

// writeAgentsFile atomically writes a root agent file, preserving the existing
// permission bits and writing through a symlink instead of replacing it.
func writeAgentsFile(path string, data []byte) error {
	target := path
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		target = resolved
	} else if !os.IsNotExist(err) {
		return err
	}
	mode := os.FileMode(0644)
	if info, err := os.Stat(target); err == nil {
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return err
	}
	return fsutil.WriteFileAtomic(target, data, mode)
}

// stripAgentsBlock rewrites path with the pylon block removed, leaving user
// content untouched. A file without a complete block is left as-is — the only
// production caller plans the strip strictly for complete blocks.
func stripAgentsBlock(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	s, e, state := findAgentsBlock(data)
	if state != agentsBlockComplete {
		return nil
	}
	out := append(append([]byte{}, data[:s]...), data[e:]...)
	return writeAgentsFile(path, out)
}

// agentsBlockIsCurrent reports whether the given block bytes carry a parseable
// version stamp at or above pylonUsageVersion.
func agentsBlockIsCurrent(blockData []byte) bool {
	m := usageVersionRe.FindSubmatch(blockData)
	if m == nil {
		return false
	}
	v, err := strconv.Atoi(string(m[1]))
	return err == nil && v >= pylonUsageVersion
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
	s, e, state := findAgentsBlock(data)
	return state != agentsBlockComplete || !agentsBlockIsCurrent(data[s:e])
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

	// AGENTS.md를 먼저 읽고 malformed면 어떤 파일도 건드리기 전에 중단한다 — CLAUDE.md를
	// 백업·교체해 놓고 에러를 반환하면 호출부가 backedUp을 버리므로 사용자는 백업 사실을
	// 통보받지 못한다.
	agentsPath := layout.RootAgentsPath(root)
	data, readErr := os.ReadFile(agentsPath)
	var s, e int
	blockState := agentsBlockAbsent
	if readErr == nil {
		s, e, blockState = findAgentsBlock(data)
		if blockState == agentsBlockMalformed {
			return false, backedUp, agentsBlockMalformedError(agentsPath)
		}
	}

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
	pointer := []byte(buildClaudeMDPointer())
	// 이미 마커면 다시 쓰지 않는다 — 매 실행 mtime 갱신으로 파일 워처를 깨우지 않기 위해.
	if existing, err := os.ReadFile(claudePath); err != nil || !bytes.Equal(existing, pointer) {
		if err := writeAgentsFile(claudePath, pointer); err != nil {
			return false, backedUp, fmt.Errorf("CLAUDE.md 생성 실패: %w", err)
		}
	}

	if blockState == agentsBlockComplete && agentsBlockIsCurrent(data[s:e]) {
		return false, backedUp, nil
	}

	block := buildAgentsBlock(root, projects)
	if readErr != nil {
		// 파일 없음 → 블록만으로 새로 만든다.
		if err := writeAgentsFile(agentsPath, []byte(block)); err != nil {
			return false, backedUp, fmt.Errorf("AGENTS.md 부트스트랩 실패: %w", err)
		}
		return true, backedUp, nil
	}

	if blockState == agentsBlockComplete {
		// stale 블록만 교체 — 블록 밖 사용자 내용은 그대로 둔다. 블록 안은 pylon 소유이고
		// 다음 세션이 팩트로부터 재저작하므로 백업하지 않는다.
		out := string(data[:s]) + block + string(data[e:])
		if err := writeAgentsFile(agentsPath, []byte(out)); err != nil {
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
		if err := writeAgentsFile(agentsPath, []byte(block)); err != nil {
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
	if err := writeAgentsFile(agentsPath, []byte(out)); err != nil {
		return false, backedUp, fmt.Errorf("AGENTS.md 부트스트랩 실패: %w", err)
	}
	return true, backedUp, nil
}
