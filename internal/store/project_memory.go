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

// keyFilePrefixExpr extracts the file portion of an incremental key
// ("<file>/<timestamp>"); keys without '/' are used as-is.
const keyFilePrefixExpr = `CASE WHEN instr(%s.key, '/') > 0 THEN substr(%s.key, 1, instr(%s.key, '/') - 1) ELSE %s.key END`

// DeleteMemory removes memory entries matching the given filters.
// projectID가 빈 문자열이면 전체 프로젝트를 대상으로 한다. key와 category는
// 비어 있지 않은 것만 조건에 포함된다. dryRun이면 삭제하지 않고 건수만 반환한다.
// FTS 인덱스는 delete 트리거로 자동 동기화된다.
func (s *Store) DeleteMemory(projectID, category, key string, dryRun bool) (int64, error) {
	var conds []string
	var args []any
	if projectID != "" {
		conds = append(conds, "project_id = ?")
		args = append(args, projectID)
	}
	if category != "" {
		conds = append(conds, "category = ?")
		args = append(args, category)
	}
	if key != "" {
		conds = append(conds, "key = ?")
		args = append(args, key)
	}
	if len(conds) == 0 {
		return 0, fmt.Errorf("at least one filter (project, category, key) is required")
	}
	where := strings.Join(conds, " AND ")

	if dryRun {
		var n int64
		err := s.db.QueryRow(`SELECT COUNT(*) FROM project_memory WHERE `+where, args...).Scan(&n)
		if err != nil {
			return 0, fmt.Errorf("failed to count memory: %w", err)
		}
		return n, nil
	}

	res, err := s.db.Exec(`DELETE FROM project_memory WHERE `+where, args...)
	if err != nil {
		return 0, fmt.Errorf("failed to delete memory: %w", err)
	}
	return res.RowsAffected()
}

// DedupMemoryChanges removes duplicate "change" entries for the same file,
// keeping only the most recent one per (project, file). projectID가 빈 문자열이면
// 전체 프로젝트를 대상으로 한다.
func (s *Store) DedupMemoryChanges(projectID string, dryRun bool) (int64, error) {
	pmPrefix := fmt.Sprintf(keyFilePrefixExpr, "pm", "pm", "pm", "pm")
	p2Prefix := fmt.Sprintf(keyFilePrefixExpr, "p2", "p2", "p2", "p2")

	where := fmt.Sprintf(`
		pm.category = 'change'
		AND (? = '' OR pm.project_id = ?)
		AND EXISTS (
			SELECT 1 FROM project_memory p2
			WHERE p2.project_id = pm.project_id
			  AND p2.category = pm.category
			  AND %s = %s
			  AND (p2.created_at > pm.created_at
			       OR (p2.created_at = pm.created_at AND p2.id > pm.id))
		)`, p2Prefix, pmPrefix)

	args := []any{projectID, projectID}

	if dryRun {
		var n int64
		err := s.db.QueryRow(`SELECT COUNT(*) FROM project_memory pm WHERE `+where, args...).Scan(&n)
		if err != nil {
			return 0, fmt.Errorf("failed to count duplicate memory: %w", err)
		}
		return n, nil
	}

	res, err := s.db.Exec(`DELETE FROM project_memory WHERE id IN (
		SELECT pm.id FROM project_memory pm WHERE `+where+`)`, args...)
	if err != nil {
		return 0, fmt.Errorf("failed to dedup memory: %w", err)
	}
	return res.RowsAffected()
}

// Vacuum reclaims unused space in the database file.
func (s *Store) Vacuum() error {
	if _, err := s.db.Exec(`VACUUM`); err != nil {
		return fmt.Errorf("failed to vacuum database: %w", err)
	}
	return nil
}

// IncrementAccessCount bumps the access count for a memory entry.
func (s *Store) IncrementAccessCount(memoryID string) error {
	_, err := s.db.Exec(`UPDATE project_memory SET access_count = access_count + 1 WHERE id = ?`, memoryID)
	if err != nil {
		return fmt.Errorf("failed to increment access count: %w", err)
	}
	return nil
}
