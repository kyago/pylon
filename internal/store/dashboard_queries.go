package store

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

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

// AdvancedMetrics holds advanced pipeline statistics for the dashboard.
type AdvancedMetrics struct {
	Throughput24h       int     `json:"throughput_24h"`
	FailureRate24h      float64 `json:"failure_rate_24h"`
	WIPCount            int     `json:"wip_count"`
	LeadTimeP50Seconds  float64 `json:"lead_time_p50_seconds"`
	LeadTimeP90Seconds  float64 `json:"lead_time_p90_seconds"`
	RetryCount          int     `json:"retry_count"`
	DLQCount            int     `json:"dlq_count"`
}

// GetAdvancedMetrics computes advanced pipeline metrics from the database.
func (s *Store) GetAdvancedMetrics() (*AdvancedMetrics, error) {
	m := &AdvancedMetrics{}

	// Throughput 24h: completed pipelines in last 24 hours
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM pipeline_state
		WHERE stage = 'completed'
		AND updated_at >= datetime('now', '-24 hours')`).Scan(&m.Throughput24h)
	if err != nil {
		return nil, fmt.Errorf("failed to compute throughput: %w", err)
	}

	// Failure rate 24h (reuse Throughput24h for completed count)
	completed24h := m.Throughput24h
	var failed24h int
	err = s.db.QueryRow(`
		SELECT COUNT(*) FROM pipeline_state
		WHERE stage = 'failed'
		AND updated_at >= datetime('now', '-24 hours')`).Scan(&failed24h)
	if err != nil {
		return nil, fmt.Errorf("failed to count failed 24h: %w", err)
	}
	terminal24h := completed24h + failed24h
	if terminal24h > 0 {
		m.FailureRate24h = float64(failed24h) / float64(terminal24h) * 100
	}

	// WIP count: active (non-terminal) pipelines
	err = s.db.QueryRow(`
		SELECT COUNT(*) FROM pipeline_state
		WHERE stage NOT IN ('completed', 'failed')`).Scan(&m.WIPCount)
	if err != nil {
		return nil, fmt.Errorf("failed to compute WIP: %w", err)
	}

	// Lead time P50/P90: computed from state_json stage_history for completed pipelines
	// We extract the duration from the first to last stage transition in the JSON
	rows, err := s.db.Query(`
		SELECT state_json FROM pipeline_state
		WHERE stage = 'completed'
		ORDER BY updated_at DESC
		LIMIT 100`)
	if err != nil {
		return nil, fmt.Errorf("failed to query lead times: %w", err)
	}
	defer rows.Close()

	var durations []float64
	for rows.Next() {
		var stateJSON string
		if err := rows.Scan(&stateJSON); err != nil {
			continue
		}
		if d := extractLeadTime(stateJSON); d > 0 {
			durations = append(durations, d)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate lead time rows: %w", err)
	}

	if len(durations) > 0 {
		sortFloat64s(durations)
		m.LeadTimeP50Seconds = percentile(durations, 50)
		m.LeadTimeP90Seconds = percentile(durations, 90)
	}

	// Retry count: sum of attempts from active pipelines
	rows2, err := s.db.Query(`
		SELECT state_json FROM pipeline_state
		WHERE stage NOT IN ('completed', 'failed')`)
	if err != nil {
		return nil, fmt.Errorf("failed to query retry counts: %w", err)
	}
	defer rows2.Close()

	for rows2.Next() {
		var stateJSON string
		if err := rows2.Scan(&stateJSON); err != nil {
			continue
		}
		m.RetryCount += extractAttempts(stateJSON)
	}
	if err := rows2.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate retry count rows: %w", err)
	}

	// DLQ count
	err = s.db.QueryRow(`SELECT COUNT(*) FROM dlq`).Scan(&m.DLQCount)
	if err != nil {
		// DLQ table might not exist yet during migration transition
		m.DLQCount = 0
	}

	return m, nil
}

// extractLeadTime parses state_json and returns the duration in seconds
// from the first to last stage transition.
func extractLeadTime(stateJSON string) float64 {
	// Quick JSON extraction without full unmarshal
	type historyEntry struct {
		CompletedAt time.Time `json:"completed_at"`
	}
	type pipelineState struct {
		History []historyEntry `json:"stage_history"`
	}

	var ps pipelineState
	if err := json.Unmarshal([]byte(stateJSON), &ps); err != nil {
		return 0
	}
	if len(ps.History) < 2 {
		return 0
	}

	first := ps.History[0].CompletedAt
	last := ps.History[len(ps.History)-1].CompletedAt
	return last.Sub(first).Seconds()
}

// extractAttempts parses state_json and returns the attempts count.
func extractAttempts(stateJSON string) int {
	type pipelineState struct {
		Attempts int `json:"attempts"`
	}
	var ps pipelineState
	if err := json.Unmarshal([]byte(stateJSON), &ps); err != nil {
		return 0
	}
	return ps.Attempts
}

// sortFloat64s sorts a slice of float64 in ascending order.
func sortFloat64s(a []float64) {
	sort.Float64s(a)
}

// percentile returns the p-th percentile of a sorted slice.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := (p / 100) * float64(len(sorted)-1)
	lower := int(idx)
	upper := lower + 1
	if upper >= len(sorted) {
		return sorted[len(sorted)-1]
	}
	frac := idx - float64(lower)
	return sorted[lower] + frac*(sorted[upper]-sorted[lower])
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
