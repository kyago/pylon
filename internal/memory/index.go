// internal/memory/index.go
package memory

import (
	"bytes"
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
		summary := strings.Join(strings.Fields(e.Content), " ")
		if runes := []rune(summary); len(runes) > 100 {
			summary = string(runes[:100]) + "…"
		}
		fmt.Fprintf(&b, "- [%s] `%s` — %s (%.1f, %s)\n",
			e.Category, e.Key, summary, e.Confidence, e.CreatedAt.Format("2006-01-02"))
	}
	return fsutil.WriteFileAtomic(indexPath, []byte(b.String()), 0644)
}

// IndexMarkdown returns the project index, truncated at a line boundary when
// maxBytes > 0. 존재하지 않는 프로젝트는 빈 문자열을 반환한다.
func (s *Store) IndexMarkdown(project string, maxBytes int) (string, error) {
	data, err := os.ReadFile(filepath.Join(s.projectDir(project), indexFileName))
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if maxBytes <= 0 || len(data) <= maxBytes {
		return string(data), nil
	}
	cut := bytes.LastIndexByte(data[:maxBytes], '\n')
	if cut <= 0 {
		return "…(생략)\n", nil
	}
	return string(data[:cut]) + "\n…(생략)\n", nil
}
