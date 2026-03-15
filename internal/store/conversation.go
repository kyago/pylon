package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// ConversationRecord represents a row in the conversations table.
type ConversationRecord struct {
	ID             string
	Title          string
	Status         string
	SessionID      string
	PipelineID     string
	Projects       string // JSON array
	TaskID         string
	AmbiguityScore float64
	ClarityScores  string // JSON object
	StartedAt      time.Time
	CompletedAt    *time.Time
	UpdatedAt      time.Time
}

// UpsertConversation inserts or updates a conversation record.
func (s *Store) UpsertConversation(rec *ConversationRecord) error {
	if err := validateConversationStatus(rec.Status); err != nil {
		return err
	}

	_, err := s.db.Exec(`
		INSERT INTO conversations (id, title, status, session_id, pipeline_id, projects, task_id, ambiguity_score, clarity_scores, started_at, completed_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			title = excluded.title,
			status = excluded.status,
			session_id = excluded.session_id,
			pipeline_id = excluded.pipeline_id,
			projects = excluded.projects,
			task_id = excluded.task_id,
			ambiguity_score = excluded.ambiguity_score,
			clarity_scores = excluded.clarity_scores,
			completed_at = excluded.completed_at,
			updated_at = excluded.updated_at`,
		rec.ID, rec.Title, rec.Status, rec.SessionID, rec.PipelineID,
		rec.Projects, rec.TaskID, rec.AmbiguityScore, rec.ClarityScores,
		rec.StartedAt, rec.CompletedAt, rec.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert conversation: %w", err)
	}
	return nil
}

// GetConversation retrieves a single conversation by ID.
func (s *Store) GetConversation(id string) (*ConversationRecord, error) {
	rec := &ConversationRecord{}
	var sessionID, pipelineID, projects, taskID, clarityScores sql.NullString
	var completedAt sql.NullTime

	err := s.db.QueryRow(`
		SELECT id, title, status, session_id, pipeline_id, projects, task_id, ambiguity_score, clarity_scores, started_at, completed_at, updated_at
		FROM conversations WHERE id = ?`, id,
	).Scan(
		&rec.ID, &rec.Title, &rec.Status, &sessionID, &pipelineID,
		&projects, &taskID, &rec.AmbiguityScore, &clarityScores,
		&rec.StartedAt, &completedAt, &rec.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get conversation: %w", err)
	}

	rec.SessionID = sessionID.String
	rec.PipelineID = pipelineID.String
	rec.Projects = projects.String
	rec.TaskID = taskID.String
	rec.ClarityScores = clarityScores.String
	if completedAt.Valid {
		rec.CompletedAt = &completedAt.Time
	}

	return rec, nil
}

// ListConversations retrieves conversations filtered by status.
// If status is empty, all conversations are returned.
func (s *Store) ListConversations(status string) ([]ConversationRecord, error) {
	if status != "" {
		if err := validateConversationStatus(status); err != nil {
			return nil, err
		}
	}

	var rows *sql.Rows
	var err error

	if status != "" {
		rows, err = s.db.Query(`
			SELECT id, title, status, session_id, pipeline_id, projects, task_id, ambiguity_score, clarity_scores, started_at, completed_at, updated_at
			FROM conversations WHERE status = ? ORDER BY updated_at DESC`, status)
	} else {
		rows, err = s.db.Query(`
			SELECT id, title, status, session_id, pipeline_id, projects, task_id, ambiguity_score, clarity_scores, started_at, completed_at, updated_at
			FROM conversations ORDER BY updated_at DESC`)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to list conversations: %w", err)
	}
	defer rows.Close()

	var result []ConversationRecord
	for rows.Next() {
		var rec ConversationRecord
		var sessionID, pipelineID, projects, taskID, clarityScores sql.NullString
		var completedAt sql.NullTime

		if err := rows.Scan(
			&rec.ID, &rec.Title, &rec.Status, &sessionID, &pipelineID,
			&projects, &taskID, &rec.AmbiguityScore, &clarityScores,
			&rec.StartedAt, &completedAt, &rec.UpdatedAt,
		); err != nil {
			return nil, err
		}

		rec.SessionID = sessionID.String
		rec.PipelineID = pipelineID.String
		rec.Projects = projects.String
		rec.TaskID = taskID.String
		rec.ClarityScores = clarityScores.String
		if completedAt.Valid {
			rec.CompletedAt = &completedAt.Time
		}

		result = append(result, rec)
	}

	return result, rows.Err()
}

// UpdateConversationStatus updates the status and optionally the completed_at timestamp.
func (s *Store) UpdateConversationStatus(id, status string, completedAt *time.Time) error {
	if err := validateConversationStatus(status); err != nil {
		return err
	}

	result, err := s.db.Exec(`
		UPDATE conversations SET status = ?, completed_at = ?, updated_at = ? WHERE id = ?`,
		status, completedAt, time.Now(), id,
	)
	if err != nil {
		return fmt.Errorf("failed to update conversation status: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("conversation %q not found", id)
	}
	return nil
}

// MarshalClarityScores converts a clarity scores map to JSON string for storage.
func MarshalClarityScores(scores map[string]float64) string {
	if len(scores) == 0 {
		return ""
	}
	data, err := json.Marshal(scores)
	if err != nil {
		return ""
	}
	return string(data)
}

// UnmarshalClarityScores converts a JSON string to a clarity scores map.
func UnmarshalClarityScores(s string) map[string]float64 {
	if s == "" {
		return nil
	}
	var scores map[string]float64
	if err := json.Unmarshal([]byte(s), &scores); err != nil {
		return nil
	}
	return scores
}
