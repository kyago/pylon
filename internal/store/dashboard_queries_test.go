package store

import (
	"testing"
	"time"
)

func TestCountMessagesByStatus(t *testing.T) {
	s := setupTestStore(t)

	// Empty DB
	counts, err := s.CountMessagesByStatus()
	if err != nil {
		t.Fatalf("CountMessagesByStatus on empty DB: %v", err)
	}
	if len(counts) != 0 {
		t.Errorf("expected empty map, got %v", counts)
	}

	// Add messages with various statuses
	msgs := []QueuedMessage{
		{ID: "m1", Type: "task_assign", FromAgent: "po", ToAgent: "arch", Body: "{}", Status: "queued", CreatedAt: time.Now()},
		{ID: "m2", Type: "result", FromAgent: "arch", ToAgent: "po", Body: "{}", Status: "queued", CreatedAt: time.Now()},
		{ID: "m3", Type: "task_assign", FromAgent: "po", ToAgent: "dev", Body: "{}", Status: "delivered", CreatedAt: time.Now()},
		{ID: "m4", Type: "result", FromAgent: "dev", ToAgent: "po", Body: "{}", Status: "acked", CreatedAt: time.Now()},
	}
	for _, msg := range msgs {
		msg := msg
		if err := s.Enqueue(&msg); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
	}

	counts, err = s.CountMessagesByStatus()
	if err != nil {
		t.Fatalf("CountMessagesByStatus: %v", err)
	}

	if counts["queued"] != 2 {
		t.Errorf("queued: want 2, got %d", counts["queued"])
	}
	if counts["delivered"] != 1 {
		t.Errorf("delivered: want 1, got %d", counts["delivered"])
	}
	if counts["acked"] != 1 {
		t.Errorf("acked: want 1, got %d", counts["acked"])
	}
}

func TestGetMessageQueueStats(t *testing.T) {
	s := setupTestStore(t)

	// Add messages
	msgs := []QueuedMessage{
		{ID: "m1", Type: "task_assign", FromAgent: "po", ToAgent: "arch", Body: "{}", Status: "queued", CreatedAt: time.Now()},
		{ID: "m2", Type: "task_assign", FromAgent: "po", ToAgent: "arch", Body: "{}", Status: "queued", CreatedAt: time.Now()},
		{ID: "m3", Type: "task_assign", FromAgent: "po", ToAgent: "dev", Body: "{}", Status: "queued", CreatedAt: time.Now()},
		{ID: "m4", Type: "result", FromAgent: "dev", ToAgent: "po", Body: "{}", Status: "acked", CreatedAt: time.Now()},
	}
	for _, msg := range msgs {
		msg := msg
		if err := s.Enqueue(&msg); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
	}

	stats, err := s.GetMessageQueueStats()
	if err != nil {
		t.Fatalf("GetMessageQueueStats: %v", err)
	}

	if len(stats) == 0 {
		t.Fatal("expected stats, got empty")
	}

	// Verify arch has 2 queued
	found := false
	for _, stat := range stats {
		if stat.ToAgent == "arch" && stat.Status == "queued" {
			found = true
			if stat.Count != 2 {
				t.Errorf("arch queued: want 2, got %d", stat.Count)
			}
		}
	}
	if !found {
		t.Error("expected arch queued stat")
	}
}

func TestGetPipelineMetrics(t *testing.T) {
	s := setupTestStore(t)

	// Empty DB
	m, err := s.GetPipelineMetrics()
	if err != nil {
		t.Fatalf("GetPipelineMetrics on empty DB: %v", err)
	}
	if m.TotalPipelines != 0 {
		t.Errorf("want 0 total, got %d", m.TotalPipelines)
	}
	if m.SuccessRate != 0 {
		t.Errorf("want 0 success rate, got %f", m.SuccessRate)
	}

	// Add pipelines
	pipelines := []PipelineRecord{
		{PipelineID: "p1", Stage: "completed", StateJSON: `{"pipeline_id":"p1","current_stage":"completed"}`, UpdatedAt: time.Now()},
		{PipelineID: "p2", Stage: "completed", StateJSON: `{"pipeline_id":"p2","current_stage":"completed"}`, UpdatedAt: time.Now()},
		{PipelineID: "p3", Stage: "failed", StateJSON: `{"pipeline_id":"p3","current_stage":"failed"}`, UpdatedAt: time.Now()},
		{PipelineID: "p4", Stage: "agent_executing", StateJSON: `{"pipeline_id":"p4","current_stage":"agent_executing"}`, UpdatedAt: time.Now()},
	}
	for _, p := range pipelines {
		if err := s.UpsertPipeline(&p); err != nil {
			t.Fatalf("UpsertPipeline: %v", err)
		}
	}

	m, err = s.GetPipelineMetrics()
	if err != nil {
		t.Fatalf("GetPipelineMetrics: %v", err)
	}

	if m.TotalPipelines != 4 {
		t.Errorf("total: want 4, got %d", m.TotalPipelines)
	}
	if m.CompletedPipelines != 2 {
		t.Errorf("completed: want 2, got %d", m.CompletedPipelines)
	}
	if m.FailedPipelines != 1 {
		t.Errorf("failed: want 1, got %d", m.FailedPipelines)
	}
	if m.ActivePipelines != 1 {
		t.Errorf("active: want 1, got %d", m.ActivePipelines)
	}

	// Success rate: 2 / (2+1) * 100 = 66.67
	expectedRate := float64(2) / float64(3) * 100
	if m.SuccessRate < expectedRate-0.1 || m.SuccessRate > expectedRate+0.1 {
		t.Errorf("success rate: want ~%.2f, got %.2f", expectedRate, m.SuccessRate)
	}
}

func TestGetPipelineMetrics_Empty(t *testing.T) {
	s := setupTestStore(t)

	m, err := s.GetPipelineMetrics()
	if err != nil {
		t.Fatalf("GetPipelineMetrics: %v", err)
	}

	if m.TotalPipelines != 0 || m.CompletedPipelines != 0 || m.FailedPipelines != 0 || m.ActivePipelines != 0 {
		t.Errorf("expected all zeros, got %+v", m)
	}
	if m.SuccessRate != 0 {
		t.Errorf("want 0 success rate, got %f", m.SuccessRate)
	}
}

func TestGetRecentMessages(t *testing.T) {
	s := setupTestStore(t)

	// Empty DB
	msgs, err := s.GetRecentMessages(10)
	if err != nil {
		t.Fatalf("GetRecentMessages on empty DB: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected empty, got %d", len(msgs))
	}

	// Add messages
	for i := 0; i < 5; i++ {
		msg := QueuedMessage{
			Type:      "task_assign",
			FromAgent: "po",
			ToAgent:   "dev",
			Body:      "{}",
			Status:    "queued",
			CreatedAt: time.Now(),
		}
		if err := s.Enqueue(&msg); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
	}

	// Get with limit
	msgs, err = s.GetRecentMessages(3)
	if err != nil {
		t.Fatalf("GetRecentMessages: %v", err)
	}
	if len(msgs) != 3 {
		t.Errorf("want 3, got %d", len(msgs))
	}

	// Default limit
	msgs, err = s.GetRecentMessages(0)
	if err != nil {
		t.Fatalf("GetRecentMessages default: %v", err)
	}
	if len(msgs) != 5 {
		t.Errorf("want 5, got %d", len(msgs))
	}
}

