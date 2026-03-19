package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// PipelineRunStatus represents the status of a pipeline in the scheduler.
type PipelineRunStatus struct {
	PipelineID string
	Status     string // running, completed, failed, waiting
	Error      error
}

// Scheduler manages concurrent pipeline execution with global resource limits.
type Scheduler struct {
	maxPipelines int
	semaphore    chan struct{}
	mu           sync.Mutex
	pipelines    map[string]*scheduledPipeline
	wg           sync.WaitGroup
	cancel       context.CancelFunc
	ctx          context.Context

	// WIP limits per stage (stage name → max concurrent pipelines at that stage)
	wipLimits map[string]int
	wipCounts map[string]int
	wipMu     sync.Mutex
}

type scheduledPipeline struct {
	id       string
	status   string // running, completed, failed, waiting
	err      error
	requires []string // pipeline IDs this depends on
}

// NewScheduler creates a Scheduler with the given concurrency limit.
// maxPipelines <= 0 defaults to 3.
func NewScheduler(maxPipelines int, wipLimits map[string]int) *Scheduler {
	if maxPipelines <= 0 {
		maxPipelines = 3
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		maxPipelines: maxPipelines,
		semaphore:    make(chan struct{}, maxPipelines),
		pipelines:    make(map[string]*scheduledPipeline),
		wipLimits:    wipLimits,
		wipCounts:    make(map[string]int),
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Submit queues a pipeline for execution. Returns immediately; execution is
// deferred if the max pipeline limit is reached. Status can be queried via Status().
// If cfg.Requires is non-empty, the pipeline waits until all required pipelines
// have completed before starting. Circular dependencies are detected eagerly.
func (s *Scheduler) Submit(cfg LoopConfig) (string, error) {
	if cfg.PipelineID == "" {
		return "", fmt.Errorf("PipelineID is required")
	}

	s.mu.Lock()
	if _, exists := s.pipelines[cfg.PipelineID]; exists {
		s.mu.Unlock()
		return "", fmt.Errorf("pipeline %s already submitted", cfg.PipelineID)
	}

	// Check for circular dependencies
	if len(cfg.Requires) > 0 {
		if err := s.detectCycleLocked(cfg.PipelineID, cfg.Requires); err != nil {
			s.mu.Unlock()
			return "", err
		}
	}

	sp := &scheduledPipeline{id: cfg.PipelineID, status: "waiting", requires: cfg.Requires}
	s.pipelines[cfg.PipelineID] = sp
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		// Recover from panics in pipeline execution (e.g., nil dependencies)
		defer func() {
			if r := recover(); r != nil {
				s.mu.Lock()
				sp.status = "failed"
				sp.err = fmt.Errorf("pipeline panic: %v", r)
				s.mu.Unlock()
			}
		}()

		// Wait for required pipelines to complete
		if len(cfg.Requires) > 0 {
			if err := s.waitForRequires(s.ctx, cfg.Requires); err != nil {
				s.mu.Lock()
				sp.status = "failed"
				sp.err = fmt.Errorf("requires wait failed: %w", err)
				s.mu.Unlock()
				return
			}
		}

		// Acquire semaphore slot (blocks if at capacity)
		select {
		case s.semaphore <- struct{}{}:
		case <-s.ctx.Done():
			s.mu.Lock()
			sp.status = "failed"
			sp.err = s.ctx.Err()
			s.mu.Unlock()
			return
		}
		defer func() { <-s.semaphore }()

		s.mu.Lock()
		sp.status = "running"
		s.mu.Unlock()

		loop := NewLoop(cfg)
		err := loop.Run(s.ctx)

		s.mu.Lock()
		if err != nil {
			sp.status = "failed"
			sp.err = err
		} else {
			sp.status = "completed"
		}
		s.mu.Unlock()
	}()

	return cfg.PipelineID, nil
}

// Stop cancels all running pipelines and waits for them to finish.
func (s *Scheduler) Stop() {
	s.cancel()
	s.wg.Wait()
}

// Status returns the current status of all submitted pipelines.
func (s *Scheduler) Status() []PipelineRunStatus {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]PipelineRunStatus, 0, len(s.pipelines))
	for _, sp := range s.pipelines {
		result = append(result, PipelineRunStatus{
			PipelineID: sp.id,
			Status:     sp.status,
			Error:      sp.err,
		})
	}
	return result
}

// Cleanup removes completed and failed pipelines from tracking to prevent memory leaks.
func (s *Scheduler) Cleanup() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := 0
	for id, sp := range s.pipelines {
		if sp.status == "completed" || sp.status == "failed" {
			delete(s.pipelines, id)
			removed++
		}
	}
	return removed
}

// RunningCount returns the number of currently running pipelines.
func (s *Scheduler) RunningCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, sp := range s.pipelines {
		if sp.status == "running" {
			count++
		}
	}
	return count
}

// AcquireWIP blocks until the pipeline can enter the given stage
// without exceeding the WIP limit. Returns nil when acquired, or
// ctx error if cancelled while waiting.
func (s *Scheduler) AcquireWIP(ctx context.Context, stage string) error {
	limit, hasLimit := s.wipLimits[stage]
	if !hasLimit || limit <= 0 {
		return nil // no limit for this stage
	}

	for {
		s.wipMu.Lock()
		if s.wipCounts[stage] < limit {
			s.wipCounts[stage]++
			s.wipMu.Unlock()
			return nil
		}
		s.wipMu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
			// retry after short wait
		}
	}
}

// ReleaseWIP decrements the WIP count for a stage.
func (s *Scheduler) ReleaseWIP(stage string) {
	s.wipMu.Lock()
	defer s.wipMu.Unlock()
	if s.wipCounts[stage] > 0 {
		s.wipCounts[stage]--
	}
}

// WIPStatus returns current WIP counts per stage.
func (s *Scheduler) WIPStatus() map[string]int {
	s.wipMu.Lock()
	defer s.wipMu.Unlock()
	result := make(map[string]int, len(s.wipCounts))
	for k, v := range s.wipCounts {
		result[k] = v
	}
	return result
}

// waitForRequires blocks until all required pipelines reach "completed" status.
// Returns error if a required pipeline fails or the context is cancelled.
func (s *Scheduler) waitForRequires(ctx context.Context, requires []string) error {
	for {
		allDone := true
		for _, reqID := range requires {
			s.mu.Lock()
			sp, exists := s.pipelines[reqID]
			if !exists {
				s.mu.Unlock()
				return fmt.Errorf("required pipeline %s not found (requires must be submitted before dependents)", reqID)
			}
			status := sp.status
			s.mu.Unlock()

			switch status {
			case "completed":
				continue
			case "failed":
				return fmt.Errorf("required pipeline %s failed", reqID)
			default:
				allDone = false
			}
		}
		if allDone {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// detectCycleLocked checks for circular dependencies among pipeline requires.
// Must be called with s.mu held.
func (s *Scheduler) detectCycleLocked(newID string, requires []string) error {
	// BFS: starting from each required pipeline, check if it can reach newID
	visited := make(map[string]bool)
	queue := make([]string, len(requires))
	copy(queue, requires)

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if current == newID {
			return fmt.Errorf("circular dependency detected: %s eventually requires itself", newID)
		}
		if visited[current] {
			continue
		}
		visited[current] = true

		if sp, exists := s.pipelines[current]; exists {
			queue = append(queue, sp.requires...)
		}
	}
	return nil
}
