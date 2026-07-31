// internal/memory/index.go

package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kyago/pylon/internal/fsutil"
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
// INDEX.md와 주입 출력이 같은 포맷을 공유한다.
func indexLine(e Entry) string {
	summary := strings.Join(strings.Fields(e.Content), " ")
	if runes := []rune(summary); len(runes) > 100 {
		summary = string(runes[:100]) + "…"
	}
	return fmt.Sprintf("- [%s] `%s` — %s (%.1f, %s)\n",
		e.Category, e.Key, summary, e.Confidence, e.CreatedAt.Format("2006-01-02"))
}

// InjectionMarkdown renders a project's entries for proactive injection,
// ranked by confidence then recency. INDEX.md(카테고리순, 사람용)와 달리
// 예산 절단이 중요도 낮은 항목부터 떨어뜨리도록 정렬한다. 예산이 한 줄도
// 담지 못하면 빈 문자열을 반환해 호출자가 예산을 다음 프로젝트로 넘기게 한다.
func (s *Store) InjectionMarkdown(project string, maxBytes int) (string, error) {
	entries, err := s.List(project)
	if err != nil || len(entries) == 0 {
		return "", err
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Confidence != entries[j].Confidence {
			return entries[i].Confidence > entries[j].Confidence
		}
		return entries[i].CreatedAt.After(entries[j].CreatedAt)
	})
	const ellipsis = "…(생략)\n"
	var b strings.Builder
	fmt.Fprintf(&b, "#### %s\n\n", project)
	wrote := 0
	truncated := false
	for _, e := range entries {
		line := indexLine(e)
		if maxBytes > 0 && b.Len()+len(line)+len(ellipsis) > maxBytes {
			truncated = true
			break
		}
		b.WriteString(line)
		wrote++
	}
	if wrote == 0 {
		return "", nil
	}
	if truncated {
		b.WriteString(ellipsis)
	}
	return b.String(), nil
}
