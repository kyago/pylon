package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

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
func (s *Store) InsertMemory(entry *MemoryEntry) error {
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

	// Update FTS index
	_, err = s.db.Exec(`
		INSERT INTO project_memory_fts (rowid, key, content, category)
		SELECT rowid, key, content, category FROM project_memory WHERE id = ?`,
		entry.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update FTS index: %w", err)
	}

	return nil
}

// SearchMemory performs BM25 full-text search on project memory.
func (s *Store) SearchMemory(projectID, query string, limit int) ([]MemorySearchResult, error) {
	if limit <= 0 {
		limit = 10
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
		query, projectID, limit,
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

// IncrementAccessCount bumps the access count for a memory entry.
func (s *Store) IncrementAccessCount(memoryID string) error {
	_, err := s.db.Exec(`UPDATE project_memory SET access_count = access_count + 1 WHERE id = ?`, memoryID)
	if err != nil {
		return fmt.Errorf("failed to increment access count: %w", err)
	}
	return nil
}
