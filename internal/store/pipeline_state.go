package store

import (
	"database/sql"
	"fmt"
	"time"
)

// PipelineRecord represents a row in the pipeline_state table.
type PipelineRecord struct {
	PipelineID string
	Stage      string
	StateJSON  string
	UpdatedAt  time.Time
}

// UpsertPipeline inserts or updates a pipeline state record.
func (s *Store) UpsertPipeline(rec *PipelineRecord) error {
	_, err := s.db.Exec(`
		INSERT INTO pipeline_state (pipeline_id, stage, state_json, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(pipeline_id) DO UPDATE SET
			stage = excluded.stage,
			state_json = excluded.state_json,
			updated_at = excluded.updated_at`,
		rec.PipelineID, rec.Stage, rec.StateJSON, rec.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert pipeline: %w", err)
	}
	return nil
}

// GetPipeline retrieves a pipeline state by ID.
func (s *Store) GetPipeline(pipelineID string) (*PipelineRecord, error) {
	rec := &PipelineRecord{}
	err := s.db.QueryRow(`
		SELECT pipeline_id, stage, state_json, updated_at
		FROM pipeline_state WHERE pipeline_id = ?`,
		pipelineID,
	).Scan(&rec.PipelineID, &rec.Stage, &rec.StateJSON, &rec.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get pipeline: %w", err)
	}
	return rec, nil
}

// GetActivePipelines returns all non-completed pipelines.
func (s *Store) GetActivePipelines() ([]PipelineRecord, error) {
	rows, err := s.db.Query(`
		SELECT pipeline_id, stage, state_json, updated_at
		FROM pipeline_state
		WHERE stage NOT IN ('completed', 'failed')
		ORDER BY updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("failed to get active pipelines: %w", err)
	}
	defer rows.Close()

	var records []PipelineRecord
	for rows.Next() {
		var rec PipelineRecord
		if err := rows.Scan(&rec.PipelineID, &rec.Stage, &rec.StateJSON, &rec.UpdatedAt); err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}
