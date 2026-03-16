package store

import "fmt"

// MessageQueueStat represents message counts grouped by agent and status.
type MessageQueueStat struct {
	ToAgent string
	Status  string
	Count   int
}

// PipelineMetrics holds aggregate pipeline statistics.
type PipelineMetrics struct {
	TotalPipelines     int
	CompletedPipelines int
	FailedPipelines    int
	ActivePipelines    int
	AvgDurationSeconds float64
	SuccessRate        float64
}

// CountMessagesByStatus returns message counts grouped by status.
func (s *Store) CountMessagesByStatus() (map[string]int, error) {
	rows, err := s.db.Query(`SELECT status, COUNT(*) FROM message_queue GROUP BY status`)
	if err != nil {
		return nil, fmt.Errorf("failed to count messages by status: %w", err)
	}
	defer rows.Close()

	result := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		result[status] = count
	}
	return result, rows.Err()
}

// GetMessageQueueStats returns message counts grouped by to_agent and status.
func (s *Store) GetMessageQueueStats() ([]MessageQueueStat, error) {
	rows, err := s.db.Query(`
		SELECT to_agent, status, COUNT(*)
		FROM message_queue
		GROUP BY to_agent, status
		ORDER BY to_agent, status`)
	if err != nil {
		return nil, fmt.Errorf("failed to get message queue stats: %w", err)
	}
	defer rows.Close()

	var stats []MessageQueueStat
	for rows.Next() {
		var stat MessageQueueStat
		if err := rows.Scan(&stat.ToAgent, &stat.Status, &stat.Count); err != nil {
			return nil, err
		}
		stats = append(stats, stat)
	}
	return stats, rows.Err()
}

// GetPipelineMetrics returns aggregate pipeline statistics.
func (s *Store) GetPipelineMetrics() (*PipelineMetrics, error) {
	m := &PipelineMetrics{}

	err := s.db.QueryRow(`SELECT COUNT(*) FROM pipeline_state`).Scan(&m.TotalPipelines)
	if err != nil {
		return nil, fmt.Errorf("failed to count pipelines: %w", err)
	}

	if m.TotalPipelines == 0 {
		return m, nil
	}

	err = s.db.QueryRow(`SELECT COUNT(*) FROM pipeline_state WHERE stage = 'completed'`).Scan(&m.CompletedPipelines)
	if err != nil {
		return nil, fmt.Errorf("failed to count completed pipelines: %w", err)
	}

	err = s.db.QueryRow(`SELECT COUNT(*) FROM pipeline_state WHERE stage = 'failed'`).Scan(&m.FailedPipelines)
	if err != nil {
		return nil, fmt.Errorf("failed to count failed pipelines: %w", err)
	}

	err = s.db.QueryRow(`SELECT COUNT(*) FROM pipeline_state WHERE stage NOT IN ('completed', 'failed')`).Scan(&m.ActivePipelines)
	if err != nil {
		return nil, fmt.Errorf("failed to count active pipelines: %w", err)
	}

	// Success rate = completed / (completed + failed) * 100
	terminal := m.CompletedPipelines + m.FailedPipelines
	if terminal > 0 {
		m.SuccessRate = float64(m.CompletedPipelines) / float64(terminal) * 100
	}

	return m, nil
}

// GetRecentMessages returns the most recent messages ordered by creation time.
func (s *Store) GetRecentMessages(limit int) ([]QueuedMessage, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.db.Query(`
		SELECT id, type, priority, from_agent, to_agent, subject, body, context, status, reply_to, ttl_seconds, created_at
		FROM message_queue
		ORDER BY created_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent messages: %w", err)
	}
	defer rows.Close()

	return scanMessages(rows)
}
