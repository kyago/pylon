package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/fsutil"
	"github.com/kyago/pylon/internal/layout"
)

// agentsBlockBegin/agentsBlockEnd delimit the pylon-managed region inside the
// workspace-root AGENTS.md. pylon regenerates what lies between the markers on every
// doctor; anything a user writes outside the block is preserved verbatim.
const agentsBlockBegin = "<!-- pylon:begin -->"
const agentsBlockEnd = "<!-- pylon:end -->"

// buildClaudeMDPointer returns the workspace-root CLAUDE.md content: a single
// Claude Code import marker that pulls AGENTS.md into the session prompt. Kept
// deterministic so doctor may rewrite it every time without churn.
func buildClaudeMDPointer() string {
	return "@AGENTS.md\n"
}

// agentsManualTitle and agentsFactsHeading frame every block buildAgentsBlock emits;
// together they identify a block as generated (see isPylonGeneratedBlock).
const (
	agentsManualTitle  = "# pylon 워크스페이스 운영 매뉴얼"
	agentsFactsHeading = "## 이 워크스페이스"
)

func isPylonClaudeMD(data []byte) bool {
	content := strings.TrimSpace(strings.ReplaceAll(string(data), "\r\n", "\n"))
	return content == strings.TrimSpace(buildClaudeMDPointer())
}

// buildAgentsBlock returns the pylon-managed block (markers included): the embedded
// usage manual verbatim, followed by the workspace facts Go knows. It is a pure
// function of the binary and the project list, so every doctor rewrites it whenever
// either changes and no version tracking is needed. The workspace root is left out on
// purpose: the same directory reached through different paths must not churn the file.
func buildAgentsBlock(projects []config.ProjectInfo) string {
	manual, _ := embeddedReference.ReadFile("reference/pylon-usage.md")
	var b strings.Builder
	b.WriteString(agentsBlockBegin + "\n")
	b.WriteString(strings.TrimSpace(string(manual)) + "\n\n")
	b.WriteString(agentsFactsHeading + "\n\n")
	b.WriteString("- **설정**: `.pylon/config.yml` / **도메인 지식**: `.pylon/domain/`\n")
	b.WriteString("- **에이전트**: `.pylon/builtin/agents/`(내장), `.pylon/agents/`(사용자 추가)\n")
	if len(projects) > 0 {
		names := make([]string, len(projects))
		for i, p := range projects {
			names[i] = p.Name
		}
		fmt.Fprintf(&b, "- **프로젝트 %d개**: %s\n", len(projects), strings.Join(names, ", "))
	} else {
		b.WriteString("- **프로젝트**: 없음 — `pylon add-project <git-url>`로 추가\n")
	}
	b.WriteString(agentsBlockEnd + "\n")
	return b.String()
}

// isPylonGeneratedBlock reports whether a block is one buildAgentsBlock emitted
// (possibly edited in place, which is reverted) and may be replaced without a backup.
// Anything else is backed up first: when unsure, keep a copy.
func isPylonGeneratedBlock(blockData []byte) bool {
	return bytes.Contains(blockData, []byte(agentsManualTitle+"\n")) &&
		bytes.Contains(blockData, []byte("\n"+agentsFactsHeading+"\n"))
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
// callers never mistake a damaged managed block for a user-owned file.
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
	info, err := os.Lstat(path)
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return err
		}
		target = resolved
	} else if err != nil && !os.IsNotExist(err) {
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
// replaced in place, so ordinary runs never churn out backups. An existing backup is
// never clobbered — the next free suffix is used. Returns the backup's file name, or ""
// when no backup was taken.
func backupIfHandWritten(path string, pylonAuthored func([]byte) bool) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil // 없으면 백업할 것도 없다
		}
		return "", fmt.Errorf("%s 읽기 실패: %w", filepath.Base(path), err)
	}
	if pylonAuthored(data) {
		return "", nil
	}
	dest, err := nextBackupPath(path)
	if err != nil {
		return "", err
	}
	if err := os.Rename(path, dest); err != nil {
		return "", fmt.Errorf("%s 백업 실패: %w", filepath.Base(path), err)
	}
	return filepath.Base(dest), nil
}

// nextBackupPath returns the first free .pylon-bak[.N] path next to path.
func nextBackupPath(path string) (string, error) {
	dest := path + rootFileBackupSuffix
	for i := 1; ; i++ {
		if _, err := os.Lstat(dest); os.IsNotExist(err) {
			return dest, nil
		}
		if i > maxRootFileBackups {
			return "", fmt.Errorf("%s 백업 실패: 백업 파일이 너무 많습니다 (%s*)",
				filepath.Base(path), filepath.Base(path)+rootFileBackupSuffix)
		}
		dest = fmt.Sprintf("%s%s.%d", path, rootFileBackupSuffix, i)
	}
}

// ensureRootAgentFiles keeps the two workspace-root files in the desired state:
// CLAUDE.md is always (re)written to the deterministic @AGENTS.md import marker, and
// the pylon block inside AGENTS.md is rewritten whenever it differs from
// buildAgentsBlock — content outside the block is NEVER modified. A user's own
// AGENTS.md (no block) keeps its content and gets the block prepended so Codex sees
// it before the default instruction-size cap. A block pylon did not generate is backed
// up to .pylon-bak before it is replaced. Returns whether AGENTS.md was written, and the
// file names of any backups taken so callers can tell the user where to look.
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
	} else if os.IsNotExist(readErr) {
		// ReadFile also reports ENOENT for a dangling symlink. Lstat distinguishes
		// that from a genuinely absent file so the link is not replaced later.
		if _, err := os.Lstat(agentsPath); err == nil {
			return false, backedUp, fmt.Errorf("AGENTS.md 읽기 실패: %w", readErr)
		} else if !os.IsNotExist(err) {
			return false, backedUp, fmt.Errorf("AGENTS.md 상태 확인 실패: %w", err)
		}
	} else {
		return false, backedUp, fmt.Errorf("AGENTS.md 읽기 실패: %w", readErr)
	}

	claudePath := layout.RootClaudePath(root)
	// pylon이 쓴 CLAUDE.md는 마커뿐이다. 그 밖의 내용은 백업한 뒤 교체한다.
	bak, err := backupIfHandWritten(claudePath, func(b []byte) bool {
		return isPylonClaudeMD(b)
	})
	if err != nil {
		return false, backedUp, err
	}
	if bak != "" {
		backedUp = append(backedUp, bak)
	}
	pointer := []byte(buildClaudeMDPointer())
	// 이미 마커면 다시 쓰지 않는다 — 매 실행 mtime 갱신으로 파일 워처를 깨우지 않기 위해.
	if existing, err := os.ReadFile(claudePath); err != nil || !bytes.Equal(existing, pointer) {
		if err := writeAgentsFile(claudePath, pointer); err != nil {
			return false, backedUp, fmt.Errorf("CLAUDE.md 생성 실패: %w", err)
		}
	}

	block := buildAgentsBlock(projects)
	var out string
	switch {
	case readErr != nil:
		// 파일 없음 → 블록만으로 새로 만든다.
		out = block
	case blockState == agentsBlockComplete:
		if string(data[s:e]) == block {
			return false, backedUp, nil // 이미 최신 — mtime을 건드리지 않는다
		}
		// 블록만 교체한다 — 블록 밖 사용자 내용은 그대로 둔다. pylon이 만든 블록이 아니면
		// 되돌릴 수 있도록 파일 전체를 .pylon-bak으로 복사해 둔다.
		if !isPylonGeneratedBlock(data[s:e]) {
			dest, err := nextBackupPath(agentsPath)
			if err != nil {
				return false, backedUp, err
			}
			mode := os.FileMode(0644)
			if info, err := os.Stat(agentsPath); err == nil {
				mode = info.Mode().Perm()
			}
			if err := fsutil.WriteFileAtomic(dest, data, mode); err != nil {
				return false, backedUp, fmt.Errorf("AGENTS.md 백업 실패: %w", err)
			}
			backedUp = append(backedUp, filepath.Base(dest))
		}
		out = string(data[:s]) + block + string(data[e:])
	default:
		// 사용자가 직접 쓴 AGENTS.md — Codex의 기본 project_doc_max_bytes(32KiB)에
		// 잘리지 않도록 블록을 앞에 두고, 기존 사용자 바이트는 그대로 뒤에 보존한다.
		out = block + string(data)
	}
	if err := writeAgentsFile(agentsPath, []byte(out)); err != nil {
		return false, backedUp, fmt.Errorf("AGENTS.md 생성 실패: %w", err)
	}
	return true, backedUp, nil
}
