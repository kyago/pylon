package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// QueuedMessage represents a row in the message_queue table.
type QueuedMessage struct {
	ID          string
	Type        string
	Priority    int
	FromAgent   string
	ToAgent     string
	Subject     string
	Body        string // JSON
	Context     string // JSON
	Status      string
	ReplyTo     string
	TTLSeconds  int
	CreatedAt   time.Time
	DeliveredAt *time.Time
	AckedAt     *time.Time
}

// Enqueue inserts a new message into the queue.
func (s *Store) Enqueue(msg *QueuedMessage) error {
	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}
	if msg.Status == "" {
		msg.Status = "queued"
	}

	_, err := s.db.Exec(`
		INSERT INTO message_queue (id, type, priority, from_agent, to_agent, subject, body, context, status, reply_to, ttl_seconds)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.ID, msg.Type, msg.Priority, msg.FromAgent, msg.ToAgent,
		msg.Subject, msg.Body, msg.Context, msg.Status, msg.ReplyTo, msg.TTLSeconds,
	)
	if err != nil {
		return fmt.Errorf("failed to enqueue message: %w", err)
	}
	return nil
}

// Dequeue retrieves and marks the next pending message for the given agent as delivered.
// Returns nil if no messages are available.
func (s *Store) Dequeue(agentName string) (*QueuedMessage, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	msg := &QueuedMessage{}
	var deliveredAt, ackedAt sql.NullTime
	var subject, context, replyTo sql.NullString

	err = tx.QueryRow(`
		SELECT id, type, priority, from_agent, to_agent, subject, body, context, status, reply_to, ttl_seconds, created_at, delivered_at, acked_at
		FROM message_queue
		WHERE to_agent = ? AND status = 'queued'
		ORDER BY priority ASC, created_at ASC
		LIMIT 1`,
		agentName,
	).Scan(
		&msg.ID, &msg.Type, &msg.Priority, &msg.FromAgent, &msg.ToAgent,
		&subject, &msg.Body, &context, &msg.Status, &replyTo,
		&msg.TTLSeconds, &msg.CreatedAt, &deliveredAt, &ackedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to dequeue: %w", err)
	}

	msg.Subject = subject.String
	msg.Context = context.String
	msg.ReplyTo = replyTo.String
	if deliveredAt.Valid {
		msg.DeliveredAt = &deliveredAt.Time
	}
	if ackedAt.Valid {
		msg.AckedAt = &ackedAt.Time
	}

	// Mark as delivered
	now := time.Now()
	_, err = tx.Exec(`UPDATE message_queue SET status = 'delivered', delivered_at = ? WHERE id = ?`, now, msg.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to mark delivered: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	msg.Status = "delivered"
	msg.DeliveredAt = &now
	return msg, nil
}

// Ack marks a message as acknowledged.
func (s *Store) Ack(messageID string) error {
	result, err := s.db.Exec(`UPDATE message_queue SET status = 'acked', acked_at = ? WHERE id = ?`, time.Now(), messageID)
	if err != nil {
		return fmt.Errorf("failed to ack message: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("message %q not found", messageID)
	}
	return nil
}

// GetByTaskID returns all messages for a given task (matching body or context JSON).
func (s *Store) GetByTaskID(taskID string) ([]QueuedMessage, error) {
	rows, err := s.db.Query(`
		SELECT id, type, priority, from_agent, to_agent, subject, body, context, status, reply_to, ttl_seconds, created_at
		FROM message_queue
		WHERE (json_valid(body) AND json_extract(body, '$.task_id') = ?)
		   OR (json_valid(context) AND json_extract(context, '$.task_id') = ?)
		ORDER BY created_at ASC`,
		taskID, taskID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query by task_id: %w", err)
	}
	defer rows.Close()

	return scanMessages(rows)
}

// GetPending returns all queued messages for a given agent.
func (s *Store) GetPending(agentName string) ([]QueuedMessage, error) {
	rows, err := s.db.Query(`
		SELECT id, type, priority, from_agent, to_agent, subject, body, context, status, reply_to, ttl_seconds, created_at
		FROM message_queue
		WHERE to_agent = ? AND status = 'queued'
		ORDER BY priority ASC, created_at ASC`,
		agentName,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending: %w", err)
	}
	defer rows.Close()

	return scanMessages(rows)
}

func scanMessages(rows *sql.Rows) ([]QueuedMessage, error) {
	var messages []QueuedMessage
	for rows.Next() {
		var msg QueuedMessage
		var subject, context, replyTo sql.NullString
		if err := rows.Scan(
			&msg.ID, &msg.Type, &msg.Priority, &msg.FromAgent, &msg.ToAgent,
			&subject, &msg.Body, &context, &msg.Status, &replyTo,
			&msg.TTLSeconds, &msg.CreatedAt,
		); err != nil {
			return nil, err
		}
		msg.Subject = subject.String
		msg.Context = context.String
		msg.ReplyTo = replyTo.String
		messages = append(messages, msg)
	}
	return messages, rows.Err()
}
