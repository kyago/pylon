package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ArchiveEntry represents a row in the session_archive table.
type ArchiveEntry struct {
	ID         string
	AgentName  string
	TaskID     string
	Summary    string
	RawPath    string
	TokenCount int
	CreatedAt  time.Time
}

// Archive stores a session archive entry.
func (s *Store) Archive(entry *ArchiveEntry) error {
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}

	_, err := s.db.Exec(`
		INSERT INTO session_archive (id, agent_name, task_id, summary, raw_path, token_count)
		VALUES (?, ?, ?, ?, ?, ?)`,
		entry.ID, entry.AgentName, entry.TaskID, entry.Summary, entry.RawPath, entry.TokenCount,
	)
	if err != nil {
		return fmt.Errorf("failed to archive session: %w", err)
	}
	return nil
}

// GetArchiveByAgent returns archives for a given agent.
func (s *Store) GetArchiveByAgent(agentName string, limit int) ([]ArchiveEntry, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(`
		SELECT id, agent_name, task_id, summary, raw_path, token_count, created_at
		FROM session_archive
		WHERE agent_name = ?
		ORDER BY created_at DESC
		LIMIT ?`,
		agentName, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get archives by agent: %w", err)
	}
	defer rows.Close()

	return scanArchives(rows)
}

// GetArchiveByTask returns archives for a given task.
func (s *Store) GetArchiveByTask(taskID string) ([]ArchiveEntry, error) {
	rows, err := s.db.Query(`
		SELECT id, agent_name, task_id, summary, raw_path, token_count, created_at
		FROM session_archive
		WHERE task_id = ?
		ORDER BY created_at DESC`,
		taskID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get archives by task: %w", err)
	}
	defer rows.Close()

	return scanArchives(rows)
}

// GetRecentArchives returns the most recent session archives.
func (s *Store) GetRecentArchives(limit int) ([]ArchiveEntry, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(`
		SELECT id, agent_name, task_id, summary, raw_path, token_count, created_at
		FROM session_archive
		ORDER BY created_at DESC
		LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent archives: %w", err)
	}
	defer rows.Close()

	return scanArchives(rows)
}

func scanArchives(rows *sql.Rows) ([]ArchiveEntry, error) {
	var entries []ArchiveEntry
	for rows.Next() {
		var e ArchiveEntry
		var rawPath sql.NullString
		if err := rows.Scan(&e.ID, &e.AgentName, &e.TaskID, &e.Summary, &rawPath, &e.TokenCount, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.RawPath = rawPath.String
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
