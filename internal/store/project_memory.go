package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
)

// fts5Operators는 FTS5 연산자 키워드 목록이다 (대소문자 무관하게 비교).
var fts5Operators = map[string]struct{}{
	"AND":  {},
	"OR":   {},
	"NOT":  {},
	"NEAR": {},
}

// sanitizeFTS5Query는 FTS5 쿼리 문자열을 안전하게 변환합니다.
// 특수 연산자를 제거하고 각 토큰을 큰따옴표로 감싸 리터럴 매칭합니다.
func sanitizeFTS5Query(query string) string {

	// 따옴표를 모두 제거하고 특수문자를 공백으로 치환
	var cleaned strings.Builder
	for _, r := range query {
		switch {
		case r == '"' || r == '\'':
			// 따옴표 제거
			cleaned.WriteRune(' ')
		case r == '*' || r == '^' || r == ':' || r == '+' || r == '(' || r == ')' || r == '{' || r == '}':
			// FTS5 특수문자를 공백으로 치환
			cleaned.WriteRune(' ')
		default:
			cleaned.WriteRune(r)
		}
	}

	// 토큰 분리 후 연산자 제외, 유효 토큰만 큰따옴표로 감싸기
	words := strings.Fields(cleaned.String())
	var tokens []string
	for _, w := range words {
		upper := strings.ToUpper(w)
		if _, isOp := fts5Operators[upper]; isOp {
			continue
		}
		// 공백이나 문자가 아닌 것만으로 이루어진 토큰은 건너뛰기
		hasContent := false
		for _, r := range w {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				hasContent = true
				break
			}
		}
		if !hasContent {
			continue
		}
		tokens = append(tokens, `"`+w+`"`)
	}

	return strings.Join(tokens, " ")
}

// MemoryEntry represents a row in the project_memory table.
type MemoryEntry struct {
	ID          string
	ProjectID   string
	Category    string
	Key         string
	Content     string
	Metadata    string // JSON
	Author      string
	Confidence  float64
	AccessCount int
	CreatedAt   time.Time
	UpdatedAt   *time.Time
	ExpiresAt   *time.Time
}

// MemorySearchResult is a search hit with BM25 rank score.
type MemorySearchResult struct {
	MemoryEntry
	Rank float64
}

// InsertMemory adds a new entry to project memory.
// FTS 인덱스는 트리거에 의해 자동으로 동기화된다.
func (s *Store) InsertMemory(entry *MemoryEntry) error {
	if err := validateConfidence(entry.Confidence); err != nil {
		return fmt.Errorf("invalid memory entry: %w", err)
	}

	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	now := time.Now()

	_, err := s.db.Exec(`
		INSERT INTO project_memory (id, project_id, category, key, content, metadata, author, confidence, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.ID, entry.ProjectID, entry.Category, entry.Key,
		entry.Content, entry.Metadata, entry.Author, entry.Confidence, now,
	)
	if err != nil {
		return fmt.Errorf("failed to insert memory: %w", err)
	}
	return nil
}

// SearchMemory performs BM25 full-text search on project memory.
func (s *Store) SearchMemory(projectID, query string, limit int) ([]MemorySearchResult, error) {
	if limit <= 0 {
		limit = 10
	}

	sanitized := sanitizeFTS5Query(query)
	if sanitized == "" {
		return nil, nil
	}

	rows, err := s.db.Query(`
		SELECT pm.id, pm.project_id, pm.category, pm.key, pm.content, pm.metadata,
		       pm.author, pm.confidence, pm.access_count, pm.created_at,
		       rank
		FROM project_memory_fts fts
		JOIN project_memory pm ON pm.rowid = fts.rowid
		WHERE project_memory_fts MATCH ?
		  AND pm.project_id = ?
		  AND (pm.expires_at IS NULL OR pm.expires_at > CURRENT_TIMESTAMP)
		ORDER BY rank
		LIMIT ?`,
		sanitized, projectID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to search memory: %w", err)
	}
	defer rows.Close()

	var results []MemorySearchResult
	for rows.Next() {
		var r MemorySearchResult
		var metadata, author sql.NullString
		if err := rows.Scan(
			&r.ID, &r.ProjectID, &r.Category, &r.Key, &r.Content, &metadata,
			&author, &r.Confidence, &r.AccessCount, &r.CreatedAt, &r.Rank,
		); err != nil {
			return nil, err
		}
		r.Metadata = metadata.String
		r.Author = author.String
		results = append(results, r)
	}
	return results, rows.Err()
}

// GetMemoryByCategory returns all entries for a project and category.
func (s *Store) GetMemoryByCategory(projectID, category string) ([]MemoryEntry, error) {
	rows, err := s.db.Query(`
		SELECT id, project_id, category, key, content, metadata, author, confidence, access_count, created_at
		FROM project_memory
		WHERE project_id = ? AND category = ?
		ORDER BY access_count DESC, created_at DESC`,
		projectID, category,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get memory by category: %w", err)
	}
	defer rows.Close()

	var entries []MemoryEntry
	for rows.Next() {
		var e MemoryEntry
		var metadata, author sql.NullString
		if err := rows.Scan(
			&e.ID, &e.ProjectID, &e.Category, &e.Key, &e.Content, &metadata,
			&author, &e.Confidence, &e.AccessCount, &e.CreatedAt,
		); err != nil {
			return nil, err
		}
		e.Metadata = metadata.String
		e.Author = author.String
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// ListProjectMemory returns all memory entries for a given project.
func (s *Store) ListProjectMemory(projectID string) ([]MemoryEntry, error) {
	rows, err := s.db.Query(`
		SELECT id, project_id, category, key, content, metadata, author, confidence, access_count, created_at
		FROM project_memory
		WHERE project_id = ?
		ORDER BY category, key`,
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list project memory: %w", err)
	}
	defer rows.Close()

	var entries []MemoryEntry
	for rows.Next() {
		var e MemoryEntry
		var metadata, author sql.NullString
		if err := rows.Scan(
			&e.ID, &e.ProjectID, &e.Category, &e.Key, &e.Content, &metadata,
			&author, &e.Confidence, &e.AccessCount, &e.CreatedAt,
		); err != nil {
			return nil, err
		}
		e.Metadata = metadata.String
		e.Author = author.String
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// IncrementAccessCount bumps the access count for a memory entry.
func (s *Store) IncrementAccessCount(memoryID string) error {
	_, err := s.db.Exec(`UPDATE project_memory SET access_count = access_count + 1 WHERE id = ?`, memoryID)
	if err != nil {
		return fmt.Errorf("failed to increment access count: %w", err)
	}
	return nil
}
