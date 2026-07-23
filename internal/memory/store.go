// internal/memory/store.go
// Package memory implements the markdown-file-backed project memory store.
// 항목 1건 = .pylon/memory/<project>/<category>/<slug>.md 파일 1개.
package memory

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kyago/pylon/internal/fsutil"
	"github.com/kyago/pylon/internal/layout"
	"github.com/kyago/pylon/internal/slug"
)

const lockTimeout = 5 * time.Second

// ErrDuplicate is returned by Insert when identical content already exists in
// the same project/category. errors.Is로 감지한다. (의사결정 D4)
var ErrDuplicate = errors.New("동일한 내용이 이미 저장되어 있습니다")

// Store is a file-based project memory store rooted at a workspace.
type Store struct {
	Root string
	Now  func() time.Time
}

func NewStore(root string) *Store {
	return &Store{Root: root, Now: time.Now}
}

func (s *Store) baseDir() string                  { return layout.MemoryDir(s.Root) }
func (s *Store) projectDir(project string) string { return layout.ProjectMemoryDir(s.Root, project) }
func (s *Store) lockPath() string                 { return filepath.Join(s.baseDir(), ".lock") }

func validateConfidence(confidence float64) error {
	if math.IsNaN(confidence) || math.IsInf(confidence, 0) || confidence < 0 || confidence > 1 {
		return fmt.Errorf("confidence는 0.0과 1.0 사이여야 합니다: %v", confidence)
	}
	return nil
}

// validateComponent rejects values unusable as a single path segment.
func validateComponent(name, value string) error {
	if value == "" || value == "." || value == ".." ||
		strings.ContainsAny(value, `/\`) || filepath.Base(value) != value {
		return fmt.Errorf("유효하지 않은 %s: %q", name, value)
	}
	return nil
}

// Insert writes a new entry as a markdown file and refreshes the project index.
func (s *Store) Insert(e *Entry) error {
	if err := validateComponent("project", e.ProjectID); err != nil {
		return err
	}
	if err := validateComponent("category", e.Category); err != nil {
		return err
	}
	if e.Key == "" || e.Content == "" {
		return fmt.Errorf("key와 content는 비어 있을 수 없습니다")
	}
	if err := validateConfidence(e.Confidence); err != nil {
		return err
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = s.Now().UTC()
	}

	if err := os.MkdirAll(s.baseDir(), 0755); err != nil {
		return err
	}
	unlock, err := fsutil.AcquireLock(s.lockPath(), lockTimeout)
	if err != nil {
		return err
	}
	defer unlock()

	// Stop hook이 매 턴 같은 학습을 보내므로 동일 내용은 저장하지 않는다 (D4).
	existing, err := s.ListByCategory(e.ProjectID, e.Category)
	if err != nil {
		return err
	}
	for _, prev := range existing {
		if prev.Content == e.Content {
			e.Path = prev.Path
			return fmt.Errorf("%w: %s", ErrDuplicate, prev.Path)
		}
	}

	dir := filepath.Join(s.projectDir(e.ProjectID), e.Category)
	name, err := availableFileName(dir, e.Key)
	if err != nil {
		return err
	}
	data, err := marshalEntry(e)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, name)
	if err := fsutil.WriteFileAtomic(path, data, 0644); err != nil {
		return err
	}
	rel, err := filepath.Rel(s.Root, path)
	if err != nil {
		return err
	}
	e.Path = filepath.ToSlash(rel)
	return s.rebuildIndexLocked(e.ProjectID)
}

// availableFileName picks a slug-based file name, suffixing -2, -3, … on collision.
func availableFileName(dir, key string) (string, error) {
	base := slug.Slugify(key)
	if runes := []rune(base); len(runes) > 80 {
		base = string(runes[:80])
	}
	for i := 1; i <= 999; i++ {
		name := base + ".md"
		if i > 1 {
			name = fmt.Sprintf("%s-%d.md", base, i)
		}
		_, err := os.Stat(filepath.Join(dir, name))
		if os.IsNotExist(err) {
			return name, nil
		}
		if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("파일 이름 충돌이 너무 많습니다: %s", base)
}

// List returns all entries for a project, sorted by category then key.
func (s *Store) List(project string) ([]Entry, error) {
	if err := validateComponent("project", project); err != nil {
		return nil, err
	}
	dir := s.projectDir(project)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, nil
	}
	var entries []Entry
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") || d.Name() == indexFileName {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		e, err := parseEntry(data)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		e.ProjectID = project
		rel, err := filepath.Rel(s.Root, path)
		if err != nil {
			return err
		}
		e.Path = filepath.ToSlash(rel)
		entries = append(entries, *e)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Category != entries[j].Category {
			return entries[i].Category < entries[j].Category
		}
		return entries[i].Key < entries[j].Key
	})
	return entries, nil
}

// ListByCategory returns one category's entries, newest first.
func (s *Store) ListByCategory(project, category string) ([]Entry, error) {
	all, err := s.List(project)
	if err != nil {
		return nil, err
	}
	var entries []Entry
	for _, e := range all {
		if e.Category == category {
			entries = append(entries, e)
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].CreatedAt.After(entries[j].CreatedAt)
	})
	return entries, nil
}

// Delete removes entries matching the filters. project가 빈 문자열이면 전체
// 프로젝트를 대상으로 한다. dryRun이면 건수만 반환한다.
func (s *Store) Delete(project, category, key string, dryRun bool) (int64, error) {
	if category == "" && key == "" {
		return 0, fmt.Errorf("category 또는 key 필터가 필요합니다")
	}
	projects := []string{project}
	if project == "" {
		dirs, err := os.ReadDir(s.baseDir())
		if os.IsNotExist(err) {
			return 0, nil
		}
		if err != nil {
			return 0, err
		}
		projects = projects[:0]
		for _, d := range dirs {
			if d.IsDir() {
				projects = append(projects, d.Name())
			}
		}
	}
	var total int64
	for _, p := range projects {
		n, err := s.deleteInProject(p, category, key, dryRun)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func (s *Store) deleteInProject(project, category, key string, dryRun bool) (int64, error) {
	entries, err := s.List(project)
	if err != nil {
		return 0, err
	}
	var targets []Entry
	for _, e := range entries {
		if category != "" && e.Category != category {
			continue
		}
		if key != "" && e.Key != key {
			continue
		}
		targets = append(targets, e)
	}
	if dryRun || len(targets) == 0 {
		return int64(len(targets)), nil
	}
	unlock, err := fsutil.AcquireLock(s.lockPath(), lockTimeout)
	if err != nil {
		return 0, err
	}
	defer unlock()
	for _, e := range targets {
		if err := os.Remove(filepath.Join(s.Root, filepath.FromSlash(e.Path))); err != nil && !os.IsNotExist(err) {
			return 0, err
		}
	}
	removeEmptyCategoryDirs(s.projectDir(project))
	return int64(len(targets)), s.rebuildIndexLocked(project)
}

// DeleteProject removes the whole memory directory of a project.
func (s *Store) DeleteProject(project string) (int64, error) {
	entries, err := s.List(project)
	if err != nil {
		return 0, err
	}
	if err := os.RemoveAll(s.projectDir(project)); err != nil {
		return 0, err
	}
	return int64(len(entries)), nil
}

// StoreLearnings saves session learnings (Stop hook 경로).
func (s *Store) StoreLearnings(project, taskID, agent string, learnings []string) error {
	for _, learning := range learnings {
		// 바이트가 아닌 룬 단위로 잘라 멀티바이트 문자가 깨지지 않게 한다.
		keyRunes := []rune(learning)
		if len(keyRunes) > 50 {
			keyRunes = keyRunes[:50]
		}
		e := &Entry{
			ProjectID:  project,
			Category:   "learning",
			Key:        fmt.Sprintf("%s/%s", taskID, sanitizeKey(string(keyRunes))),
			Content:    learning,
			Author:     agent,
			Confidence: 0.8,
		}
		if err := s.Insert(e); err != nil {
			if errors.Is(err, ErrDuplicate) {
				continue // 매 턴 반복되는 동일 학습은 조용히 건너뛴다 (D4)
			}
			return fmt.Errorf("학습 내용 저장 실패: %w", err)
		}
	}
	return nil
}

func sanitizeKey(v string) string {
	return strings.NewReplacer(" ", "-", "/", "-", ":", "-").Replace(v)
}

// removeEmptyCategoryDirs prunes category directories emptied by deletion.
func removeEmptyCategoryDirs(projectDir string) {
	dirs, err := os.ReadDir(projectDir)
	if err != nil {
		return
	}
	for _, d := range dirs {
		if d.IsDir() {
			_ = os.Remove(filepath.Join(projectDir, d.Name())) // 비어 있을 때만 성공
		}
	}
}
