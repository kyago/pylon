package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// BlackboardEntry represents a row in the blackboard table.
type BlackboardEntry struct {
	ID           string
	ProjectID    string
	Category     string
	Key          string
	Value        string // JSON
	Confidence   float64
	Author       string
	CreatedAt    time.Time
	UpdatedAt    *time.Time
	SupersededBy string
}

// PutBlackboard inserts or updates a blackboard entry.
// If a matching (project_id, category, key) exists, it supersedes the old entry.
func (s *Store) PutBlackboard(entry *BlackboardEntry) error {
	if err := validateConfidence(entry.Confidence); err != nil {
		return fmt.Errorf("invalid blackboard entry: %w", err)
	}

	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	now := time.Now()

	_, err := s.db.Exec(`
		INSERT INTO blackboard (id, project_id, category, key, value, confidence, author, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(project_id, category, key) DO UPDATE SET
			value = excluded.value,
			confidence = excluded.confidence,
			author = excluded.author,
			updated_at = ?`,
		entry.ID, entry.ProjectID, entry.Category, entry.Key,
		entry.Value, entry.Confidence, entry.Author, now, now, now,
	)
	if err != nil {
		return fmt.Errorf("failed to put blackboard entry: %w", err)
	}
	return nil
}

// GetBlackboard retrieves a blackboard entry by project, category, and key.
func (s *Store) GetBlackboard(projectID, category, key string) (*BlackboardEntry, error) {
	entry := &BlackboardEntry{}
	var updatedAt sql.NullTime
	var supersededBy sql.NullString

	err := s.db.QueryRow(`
		SELECT id, project_id, category, key, value, confidence, author, created_at, updated_at, superseded_by
		FROM blackboard
		WHERE project_id = ? AND category = ? AND key = ?`,
		projectID, category, key,
	).Scan(
		&entry.ID, &entry.ProjectID, &entry.Category, &entry.Key,
		&entry.Value, &entry.Confidence, &entry.Author,
		&entry.CreatedAt, &updatedAt, &supersededBy,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get blackboard entry: %w", err)
	}
	if updatedAt.Valid {
		entry.UpdatedAt = &updatedAt.Time
	}
	entry.SupersededBy = supersededBy.String
	return entry, nil
}

// GetBlackboardByCategory returns all entries for a project and category.
func (s *Store) GetBlackboardByCategory(projectID, category string) ([]BlackboardEntry, error) {
	rows, err := s.db.Query(`
		SELECT id, project_id, category, key, value, confidence, author, created_at
		FROM blackboard
		WHERE project_id = ? AND category = ?
		ORDER BY created_at DESC`,
		projectID, category,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query blackboard: %w", err)
	}
	defer rows.Close()

	var entries []BlackboardEntry
	for rows.Next() {
		var e BlackboardEntry
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.Category, &e.Key, &e.Value, &e.Confidence, &e.Author, &e.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
