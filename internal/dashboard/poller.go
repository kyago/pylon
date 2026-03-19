package dashboard

import (
	"context"
	"io"
	"log"
	"time"

	"github.com/kyago/pylon/internal/orchestrator"
)

// pipelineSnapshot holds the state for change detection.
type pipelineSnapshot struct {
	Stage       string
	Status      string
	AgentStates map[string]string
}

// Poller periodically queries the store and publishes SSE events on changes.
type Poller struct {
	store          DashboardStore
	hub            *SSEHub
	logger         *log.Logger
	prev           map[string]pipelineSnapshot
	knownTerminals map[string]bool // terminal 상태 도달 후 재감지 방지
}

// NewPoller creates a new poller.
func NewPoller(s DashboardStore, hub *SSEHub, logger *log.Logger) *Poller {
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}
	return &Poller{
		store:          s,
		hub:            hub,
		logger:         logger,
		prev:           make(map[string]pipelineSnapshot),
		knownTerminals: make(map[string]bool),
	}
}

// Run starts the 1-second polling loop.
func (p *Poller) Run(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.poll()
		}
	}
}

func (p *Poller) poll() {
	records, err := p.store.ListAllPipelines()
	if err != nil {
		p.logger.Printf("poller: failed to list pipelines: %v", err)
		return
	}

	current := make(map[string]pipelineSnapshot)
	var runningAgents int

	for _, rec := range records {
		pipeline, err := orchestrator.LoadPipeline([]byte(rec.StateJSON))
		if err != nil {
			continue
		}

		snap := pipelineSnapshot{
			Stage:       rec.Stage,
			Status:      rec.Status,
			AgentStates: make(map[string]string),
		}

		for name, agent := range pipeline.Agents {
			snap.AgentStates[name] = agent.Status
			if agent.Status == "running" {
				runningAgents++
			}
		}

		current[rec.PipelineID] = snap

		prev, existed := p.prev[rec.PipelineID]
		if !existed {
			// 이미 completion 이벤트를 발행한 terminal 파이프라인은 건너뜀
			if p.knownTerminals[rec.PipelineID] {
				continue
			}
			p.hub.Publish(SSEEvent{
				Type: "pipeline_created",
				Data: map[string]any{
					"pipeline_id": rec.PipelineID,
					"stage":       rec.Stage,
				},
			})
			continue
		}

		// Status change (pause/resume)
		if prev.Status != snap.Status {
			if snap.Status == "paused" {
				p.hub.Publish(SSEEvent{
					Type: "pipeline_paused",
					Data: map[string]any{
						"pipeline_id": rec.PipelineID,
						"stage":       snap.Stage,
						"paused_at":   time.Now().Format(time.RFC3339),
					},
				})
			} else if prev.Status == "paused" && snap.Status == "running" {
				p.hub.Publish(SSEEvent{
					Type: "pipeline_resumed",
					Data: map[string]any{
						"pipeline_id": rec.PipelineID,
						"stage":       snap.Stage,
						"resumed_at":  time.Now().Format(time.RFC3339),
					},
				})
			}
		}

		// Stage change
		if prev.Stage != snap.Stage {
			eventType := "pipeline_stage_change"
			if snap.Stage == "completed" || snap.Stage == "failed" {
				eventType = "pipeline_completed"
			}
			p.hub.Publish(SSEEvent{
				Type: eventType,
				Data: map[string]any{
					"pipeline_id": rec.PipelineID,
					"from":        prev.Stage,
					"to":          snap.Stage,
				},
			})
		}

		// Agent status change
		for name, status := range snap.AgentStates {
			if prevStatus, ok := prev.AgentStates[name]; !ok || prevStatus != status {
				p.hub.Publish(SSEEvent{
					Type: "agent_status_change",
					Data: map[string]any{
						"pipeline_id": rec.PipelineID,
						"agent":       name,
						"status":      status,
					},
				})
			}
		}
	}

	// Check for running agent count change
	var prevRunning int
	for _, snap := range p.prev {
		for _, status := range snap.AgentStates {
			if status == "running" {
				prevRunning++
			}
		}
	}

	if runningAgents != prevRunning {
		p.hub.Publish(SSEEvent{
			Type: "concurrency_update",
			Data: map[string]any{
				"running_agents": runningAgents,
			},
		})
	}

	// Terminal 파이프라인은 knownTerminals에 기록하고 prev에서 제거하여
	// 메모리 누적을 방지한다. knownTerminals가 있으므로 다음 poll에서
	// pipeline_created 이벤트가 재발행되지 않는다.
	for id, snap := range current {
		if snap.Stage == "completed" || snap.Stage == "failed" {
			p.knownTerminals[id] = true
			delete(current, id)
		}
	}

	p.prev = current
}
