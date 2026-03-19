package store

import (
	"database/sql"
	"fmt"
	"time"
)

// DLQEntry represents a row in the dlq table.
type DLQEntry struct {
	ID               int
	PipelineID       string
	WorkflowName     string
	Stage            string
	ErrorMessage     string
	ErrorOutput      string
	OriginalStateJSON string
	CreatedAt        time.Time
}

// InsertDLQ adds a failed pipeline entry to the dead letter queue.
func (s *Store) InsertDLQ(entry *DLQEntry) error {
	_, err := s.db.Exec(`
		INSERT INTO dlq (pipeline_id, workflow_name, stage, error_message, error_output, original_state_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		entry.PipelineID, entry.WorkflowName, entry.Stage, entry.ErrorMessage, entry.ErrorOutput, entry.OriginalStateJSON, time.Now(),
	)
	if err != nil {
		return fmt.Errorf("failed to insert DLQ entry: %w", err)
	}
	return nil
}

// ListDLQ returns all DLQ entries ordered by creation time descending.
func (s *Store) ListDLQ() ([]DLQEntry, error) {
	rows, err := s.db.Query(`
		SELECT id, pipeline_id, workflow_name, stage, error_message, error_output, original_state_json, created_at
		FROM dlq
		ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("failed to list DLQ entries: %w", err)
	}
	defer rows.Close()

	return scanDLQEntries(rows)
}

// GetDLQEntry retrieves a single DLQ entry by ID.
func (s *Store) GetDLQEntry(id int) (*DLQEntry, error) {
	entry := &DLQEntry{}
	err := s.db.QueryRow(`
		SELECT id, pipeline_id, workflow_name, stage, error_message, error_output, original_state_json, created_at
		FROM dlq WHERE id = ?`, id,
	).Scan(&entry.ID, &entry.PipelineID, &entry.WorkflowName, &entry.Stage,
		&entry.ErrorMessage, &entry.ErrorOutput, &entry.OriginalStateJSON, &entry.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get DLQ entry: %w", err)
	}
	return entry, nil
}

// DeleteDLQEntry removes a DLQ entry by ID (dismiss).
func (s *Store) DeleteDLQEntry(id int) error {
	result, err := s.db.Exec(`DELETE FROM dlq WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete DLQ entry: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("DLQ entry %d not found", id)
	}
	return nil
}

// RequeueDLQ moves a DLQ entry back to the pipeline_state table for retry.
// It re-inserts the pipeline with its original state and removes the DLQ entry.
func (s *Store) RequeueDLQ(id int) error {
	entry, err := s.GetDLQEntry(id)
	if err != nil {
		return err
	}
	if entry == nil {
		return fmt.Errorf("DLQ entry %d not found", id)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Re-insert into pipeline_state with the original state
	_, err = tx.Exec(`
		INSERT INTO pipeline_state (pipeline_id, stage, state_json, workflow_name, status, paused_at_stage, updated_at)
		VALUES (?, ?, ?, ?, 'running', '', ?)
		ON CONFLICT(pipeline_id) DO UPDATE SET
			stage = excluded.stage,
			state_json = excluded.state_json,
			workflow_name = excluded.workflow_name,
			status = excluded.status,
			paused_at_stage = excluded.paused_at_stage,
			updated_at = excluded.updated_at`,
		entry.PipelineID, entry.Stage, entry.OriginalStateJSON, entry.WorkflowName, time.Now(),
	)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to requeue pipeline: %w", err)
	}

	// Remove from DLQ
	_, err = tx.Exec(`DELETE FROM dlq WHERE id = ?`, id)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to remove DLQ entry: %w", err)
	}

	return tx.Commit()
}

// CountDLQ returns the number of entries in the DLQ.
func (s *Store) CountDLQ() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM dlq`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count DLQ entries: %w", err)
	}
	return count, nil
}

func scanDLQEntries(rows *sql.Rows) ([]DLQEntry, error) {
	var entries []DLQEntry
	for rows.Next() {
		var e DLQEntry
		if err := rows.Scan(&e.ID, &e.PipelineID, &e.WorkflowName, &e.Stage,
			&e.ErrorMessage, &e.ErrorOutput, &e.OriginalStateJSON, &e.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
