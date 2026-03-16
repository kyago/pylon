package store

import (
	"fmt"
	"time"
)

// ProjectRecord represents a row in the projects table.
type ProjectRecord struct {
	ProjectID string
	Path      string
	Stack     string
	CreatedAt time.Time
}

// UpsertProject inserts or updates a project record.
func (s *Store) UpsertProject(rec *ProjectRecord) error {
	_, err := s.db.Exec(`
		INSERT INTO projects (project_id, path, stack)
		VALUES (?, ?, ?)
		ON CONFLICT(project_id) DO UPDATE SET
			path = excluded.path,
			stack = excluded.stack`,
		rec.ProjectID, rec.Path, rec.Stack,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert project: %w", err)
	}
	return nil
}

// ListProjects returns all registered projects.
func (s *Store) ListProjects() ([]ProjectRecord, error) {
	rows, err := s.db.Query(`
		SELECT project_id, path, stack, created_at
		FROM projects
		ORDER BY project_id`)
	if err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}
	defer rows.Close()

	var projects []ProjectRecord
	for rows.Next() {
		var p ProjectRecord
		if err := rows.Scan(&p.ProjectID, &p.Path, &p.Stack, &p.CreatedAt); err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}
