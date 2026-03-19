package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// WorkerStatus holds the current and max concurrent count for a model.
type WorkerStatus struct {
	Model   string `json:"model"`
	Active  int    `json:"active"`
	MaxSlot int    `json:"max_slot"`
}

// WorkerPool tracks per-model concurrency for agent execution.
// It enforces limits on how many agents of each model can run simultaneously.
type WorkerPool struct {
	limits map[string]int // model → max concurrent
	active map[string]int // model → current running count
	mu     sync.Mutex
}

// NewWorkerPool creates a WorkerPool with the given per-model limits.
// Models not in the limits map have no concurrency restriction.
func NewWorkerPool(limits map[string]int) *WorkerPool {
	if limits == nil {
		limits = make(map[string]int)
	}
	return &WorkerPool{
		limits: limits,
		active: make(map[string]int),
	}
}

// Acquire blocks until a slot is available for the given model.
// Returns nil when the slot is acquired, or ctx.Err() if cancelled.
// Models without a configured limit are always granted immediately.
func (wp *WorkerPool) Acquire(ctx context.Context, model string) error {
	limit, hasLimit := wp.limits[model]
	if !hasLimit || limit <= 0 {
		wp.mu.Lock()
		wp.active[model]++
		wp.mu.Unlock()
		return nil
	}

	for {
		wp.mu.Lock()
		if wp.active[model] < limit {
			wp.active[model]++
			wp.mu.Unlock()
			return nil
		}
		wp.mu.Unlock()

		select {
		case <-ctx.Done():
			return fmt.Errorf("worker pool acquire cancelled for model %s: %w", model, ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// Release returns a slot for the given model.
func (wp *WorkerPool) Release(model string) {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	if wp.active[model] > 0 {
		wp.active[model]--
	}
}

// Status returns the current worker status for all tracked models.
func (wp *WorkerPool) Status() map[string]WorkerStatus {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	result := make(map[string]WorkerStatus)

	// Include all models with limits
	for model, limit := range wp.limits {
		result[model] = WorkerStatus{
			Model:   model,
			Active:  wp.active[model],
			MaxSlot: limit,
		}
	}

	// Include active models without explicit limits
	for model, count := range wp.active {
		if _, exists := result[model]; !exists {
			result[model] = WorkerStatus{
				Model:   model,
				Active:  count,
				MaxSlot: 0, // 0 = unlimited
			}
		}
	}

	return result
}

// TotalActive returns the total number of active workers across all models.
func (wp *WorkerPool) TotalActive() int {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	total := 0
	for _, count := range wp.active {
		total += count
	}
	return total
}
