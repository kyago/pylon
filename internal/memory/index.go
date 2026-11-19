// internal/memory/index.go

package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kyago/pylon/internal/fsutil"
	"github.com/kyago/pylon/internal/layout"
)

const indexFileName = "INDEX.md"

// rebuildIndexLocked regenerates INDEX.md. Caller must hold the store lock.
func (s *Store) rebuildIndexLocked(project string) error {
	entries, err := s.List(project)
	if err != nil {
		return err
	}
	indexPath := filepath.Join(s.projectDir(project), indexFileName)
	if len(entries) == 0 {
		if err := os.Remove(indexPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Category != entries[j].Category {
			return entries[i].Category < entries[j].Category
		}
		return entries[i].CreatedAt.After(entries[j].CreatedAt)
	})
	var b strings.Builder
	fmt.Fprintf(&b, "# %s 프로젝트 메모리 인덱스\n\n", project)
	for _, e := range entries {
		b.WriteString(indexLine(e))
	}
	return fsutil.WriteFileAtomic(indexPath, []byte(b.String()), 0644)
}

// indexLine renders one entry as an index/injection line.
// INDEX.md와 주입 출력이 같은 포맷을 공유한다. 요약은 본문의 첫 비어 있지
// 않은 줄이다 — 첫 줄 = 한 줄 요약 관례(마크다운 제목이면 # 표시를 뗀다).
// 줄 끝에는 .pylon/memory/ 기준 전문 파일 경로를 붙인다.
func indexLine(e Entry) string {
	var summary string
	for _, line := range strings.Split(e.Content, "\n") {
		line = strings.TrimLeft(strings.TrimSpace(line), "#")
		if summary = strings.Join(strings.Fields(line), " "); summary != "" {
			break
		}
	}
	if runes := []rune(summary); len(runes) > 200 {
		summary = string(runes[:200]) + "…"
	}
	path := strings.TrimPrefix(e.Path, filepath.ToSlash(layout.MemoryDir(""))+"/")
	return fmt.Sprintf("- [%s] `%s` — %s (%.1f, %s) → %s\n",
		e.Category, e.Key, summary, e.Confidence, e.CreatedAt.Format("2006-01-02"), path)
}
