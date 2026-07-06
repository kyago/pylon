package store

import (
	"database/sql"
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

// GetProject returns a single project by ID. Returns sql.ErrNoRows (wrapped)
// when the project is not registered.
func (s *Store) GetProject(projectID string) (*ProjectRecord, error) {
	var p ProjectRecord
	err := s.db.QueryRow(`
		SELECT project_id, path, stack, created_at
		FROM projects
		WHERE project_id = ?`, projectID,
	).Scan(&p.ProjectID, &p.Path, &p.Stack, &p.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("project %q is not registered", projectID)
		}
		return nil, fmt.Errorf("failed to get project: %w", err)
	}
	return &p, nil
}

// DeleteProjectResult reports how many rows were removed per table.
type DeleteProjectResult struct {
	Projects int64
	Memory   int64
}

// DeleteProject removes a project registration together with its associated
// project_memory rows in a single transaction.
func (s *Store) DeleteProject(projectID string) (DeleteProjectResult, error) {
	var res DeleteProjectResult

	tx, err := s.db.Begin()
	if err != nil {
		return res, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	memRes, err := tx.Exec(`DELETE FROM project_memory WHERE project_id = ?`, projectID)
	if err != nil {
		return res, fmt.Errorf("failed to delete project_memory: %w", err)
	}
	res.Memory, _ = memRes.RowsAffected()

	projRes, err := tx.Exec(`DELETE FROM projects WHERE project_id = ?`, projectID)
	if err != nil {
		return res, fmt.Errorf("failed to delete project: %w", err)
	}
	res.Projects, _ = projRes.RowsAffected()

	if err := tx.Commit(); err != nil {
		return res, fmt.Errorf("failed to commit transaction: %w", err)
	}
	return res, nil
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
