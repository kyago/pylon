package dashboard

import (
	"context"
	"testing"
	"time"

	"github.com/kyago/pylon/internal/store"
)

func TestPollerDetectsNewPipeline(t *testing.T) {
	mock := &mockStore{}
	hub := NewSSEHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	poller := NewPoller(mock, hub, nil)

	ch, unsub := hub.Subscribe()
	defer unsub()
	time.Sleep(10 * time.Millisecond)

	// Add a pipeline and poll
	mock.pipelines = []store.PipelineRecord{
		{PipelineID: "p1", Stage: "init", StateJSON: `{"pipeline_id":"p1","current_stage":"init","created_at":"2025-01-01T00:00:00Z"}`, UpdatedAt: time.Now()},
	}
	poller.poll()

	select {
	case event := <-ch:
		if event.Type != "pipeline_created" {
			t.Errorf("want pipeline_created, got %s", event.Type)
		}
		data := event.Data.(map[string]any)
		if data["pipeline_id"] != "p1" {
			t.Errorf("want p1, got %v", data["pipeline_id"])
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for pipeline_created event")
	}
}

func TestPollerDetectsStageChange(t *testing.T) {
	mock := &mockStore{
		pipelines: []store.PipelineRecord{
			{PipelineID: "p1", Stage: "init", StateJSON: `{"pipeline_id":"p1","current_stage":"init","created_at":"2025-01-01T00:00:00Z"}`, UpdatedAt: time.Now()},
		},
	}
	hub := NewSSEHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	poller := NewPoller(mock, hub, nil)

	// First poll to establish baseline
	poller.poll()

	ch, unsub := hub.Subscribe()
	defer unsub()
	time.Sleep(10 * time.Millisecond)

	// Change stage
	mock.pipelines[0].Stage = "po_conversation"
	mock.pipelines[0].StateJSON = `{"pipeline_id":"p1","current_stage":"po_conversation","created_at":"2025-01-01T00:00:00Z"}`

	poller.poll()

	select {
	case event := <-ch:
		if event.Type != "pipeline_stage_change" {
			t.Errorf("want pipeline_stage_change, got %s", event.Type)
		}
		data := event.Data.(map[string]any)
		if data["from"] != "init" {
			t.Errorf("want from=init, got %v", data["from"])
		}
		if data["to"] != "po_conversation" {
			t.Errorf("want to=po_conversation, got %v", data["to"])
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for stage change event")
	}
}

func TestPollerDetectsCompletion(t *testing.T) {
	mock := &mockStore{
		pipelines: []store.PipelineRecord{
			{PipelineID: "p1", Stage: "wiki_update", StateJSON: `{"pipeline_id":"p1","current_stage":"wiki_update","created_at":"2025-01-01T00:00:00Z"}`, UpdatedAt: time.Now()},
		},
	}
	hub := NewSSEHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	poller := NewPoller(mock, hub, nil)
	poller.poll()

	ch, unsub := hub.Subscribe()
	defer unsub()
	time.Sleep(10 * time.Millisecond)

	mock.pipelines[0].Stage = "completed"
	mock.pipelines[0].StateJSON = `{"pipeline_id":"p1","current_stage":"completed","created_at":"2025-01-01T00:00:00Z"}`

	poller.poll()

	select {
	case event := <-ch:
		if event.Type != "pipeline_completed" {
			t.Errorf("want pipeline_completed, got %s", event.Type)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for pipeline_completed event")
	}
}

func TestPollerNoEventsOnNoChange(t *testing.T) {
	mock := &mockStore{
		pipelines: []store.PipelineRecord{
			{PipelineID: "p1", Stage: "init", StateJSON: `{"pipeline_id":"p1","current_stage":"init","created_at":"2025-01-01T00:00:00Z"}`, UpdatedAt: time.Now()},
		},
	}
	hub := NewSSEHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	poller := NewPoller(mock, hub, nil)

	// Establish baseline
	poller.poll()

	ch, unsub := hub.Subscribe()
	defer unsub()
	time.Sleep(10 * time.Millisecond)

	// Poll again with no changes
	poller.poll()

	select {
	case event := <-ch:
		t.Errorf("unexpected event: %s", event.Type)
	case <-time.After(100 * time.Millisecond):
		// Good - no events
	}
}

func TestPollerConcurrencyUpdate(t *testing.T) {
	mock := &mockStore{}
	hub := NewSSEHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	poller := NewPoller(mock, hub, nil)

	// Establish empty baseline
	poller.poll()

	ch, unsub := hub.Subscribe()
	defer unsub()
	time.Sleep(10 * time.Millisecond)

	// Add pipeline with running agent
	mock.pipelines = []store.PipelineRecord{
		{
			PipelineID: "p1",
			Stage:      "agent_executing",
			StateJSON:  `{"pipeline_id":"p1","current_stage":"agent_executing","active_agents":{"dev":{"task_id":"t1","agent_id":"a1","status":"running"}},"created_at":"2025-01-01T00:00:00Z"}`,
			UpdatedAt:  time.Now(),
		},
	}
	poller.poll()

	// Should get pipeline_created and concurrency_update
	events := drainEvents(ch, 500*time.Millisecond)
	found := false
	for _, e := range events {
		if e.Type == "concurrency_update" {
			data := e.Data.(map[string]any)
			if data["running_agents"].(int) != 1 {
				t.Errorf("want running_agents=1, got %v", data["running_agents"])
			}
			found = true
		}
	}
	if !found {
		t.Error("concurrency_update event not found")
	}
}

func TestPollerNoSpuriousCreatedForTerminal(t *testing.T) {
	// 회귀 테스트: terminal 파이프라인이 매초 pipeline_created를 재발행하지 않아야 함
	mock := &mockStore{
		pipelines: []store.PipelineRecord{
			{PipelineID: "p1", Stage: "wiki_update", StateJSON: `{"pipeline_id":"p1","current_stage":"wiki_update","created_at":"2025-01-01T00:00:00Z"}`, UpdatedAt: time.Now()},
		},
	}
	hub := NewSSEHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	poller := NewPoller(mock, hub, nil)

	// 첫 poll: pipeline_created 발행
	poller.poll()

	ch, unsub := hub.Subscribe()
	defer unsub()
	time.Sleep(10 * time.Millisecond)

	// 파이프라인 완료
	mock.pipelines[0].Stage = "completed"
	mock.pipelines[0].StateJSON = `{"pipeline_id":"p1","current_stage":"completed","created_at":"2025-01-01T00:00:00Z"}`

	// 두번째 poll: pipeline_completed 발행
	poller.poll()
	events := drainEvents(ch, 200*time.Millisecond)
	foundCompleted := false
	for _, e := range events {
		if e.Type == "pipeline_completed" {
			foundCompleted = true
		}
	}
	if !foundCompleted {
		t.Error("expected pipeline_completed event")
	}

	// 세번째 poll: 같은 terminal 파이프라인에 대해 이벤트가 없어야 함
	poller.poll()
	events = drainEvents(ch, 200*time.Millisecond)
	for _, e := range events {
		if e.Type == "pipeline_created" || e.Type == "pipeline_completed" {
			t.Errorf("unexpected spurious event after terminal: %s", e.Type)
		}
	}
}

func drainEvents(ch chan SSEEvent, timeout time.Duration) []SSEEvent {
	var events []SSEEvent
	timer := time.After(timeout)
	for {
		select {
		case e := <-ch:
			events = append(events, e)
		case <-timer:
			return events
		}
	}
}
