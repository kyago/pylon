package orchestrator

import (
	"fmt"
	"time"
)

// TaskItem represents a single task in the dependency graph.
type TaskItem struct {
	ID           string     `json:"id"`
	Description  string     `json:"description"`
	AgentName    string     `json:"agent_name,omitempty"`
	DependsOn    []string   `json:"depends_on,omitempty"`
	BlockedBy    []string   `json:"blocked_by,omitempty"` // runtime dependencies resolved dynamically
	Status       string     `json:"status,omitempty"`     // pending, running, completed, failed
	StartedAt    *time.Time `json:"started_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	ErrorMessage string     `json:"error_message,omitempty"`
	FileCount    int        `json:"file_count,omitempty"`
}

// IsBlocked returns true if any BlockedBy task is not yet completed.
func (t *TaskItem) IsBlocked(completedTasks map[string]bool) bool {
	for _, blockerID := range t.BlockedBy {
		if !completedTasks[blockerID] {
			return true
		}
	}
	return false
}

// TaskGraph holds tasks with dependency information for wave-based execution.
type TaskGraph struct {
	Tasks []TaskItem `json:"tasks"`
}

// Validate checks for cycles and dangling dependency references.
func (g *TaskGraph) Validate() error {
	ids := make(map[string]bool, len(g.Tasks))
	for _, t := range g.Tasks {
		if t.ID == "" {
			return fmt.Errorf("task has empty ID")
		}
		if ids[t.ID] {
			return fmt.Errorf("duplicate task ID: %s", t.ID)
		}
		ids[t.ID] = true
	}

	for _, t := range g.Tasks {
		for _, dep := range t.DependsOn {
			if !ids[dep] {
				return fmt.Errorf("task %s depends on unknown ID: %s", t.ID, dep)
			}
		}
		for _, blocker := range t.BlockedBy {
			if !ids[blocker] {
				return fmt.Errorf("task %s blocked_by unknown ID: %s", t.ID, blocker)
			}
		}
	}

	// Cycle detection via topological sort attempt
	_, err := g.TopoSort()
	return err
}

// TopoSort returns tasks grouped into execution waves using Kahn's algorithm.
// Wave 0 contains tasks with no dependencies, Wave 1 depends on Wave 0, etc.
func (g *TaskGraph) TopoSort() ([][]TaskItem, error) {
	taskMap := make(map[string]*TaskItem, len(g.Tasks))
	inDegree := make(map[string]int, len(g.Tasks))
	dependents := make(map[string][]string) // dep → tasks that depend on it

	for i := range g.Tasks {
		t := &g.Tasks[i]
		taskMap[t.ID] = t
		inDegree[t.ID] = len(t.DependsOn)
		for _, dep := range t.DependsOn {
			dependents[dep] = append(dependents[dep], t.ID)
		}
	}

	// Seed with zero-degree tasks in original g.Tasks order for deterministic results
	var queue []string
	for _, t := range g.Tasks {
		if inDegree[t.ID] == 0 {
			queue = append(queue, t.ID)
		}
	}

	var waves [][]TaskItem
	processed := 0

	for len(queue) > 0 {
		wave := make([]TaskItem, 0, len(queue))
		var nextQueue []string

		for _, id := range queue {
			wave = append(wave, *taskMap[id])
			processed++

			for _, depID := range dependents[id] {
				inDegree[depID]--
				if inDegree[depID] == 0 {
					nextQueue = append(nextQueue, depID)
				}
			}
		}

		waves = append(waves, wave)
		queue = nextQueue
	}

	if processed != len(g.Tasks) {
		return nil, fmt.Errorf("cyclic dependency detected: %d of %d tasks processed", processed, len(g.Tasks))
	}

	return waves, nil
}

// AssignAgents assigns agent names to tasks that have no agent_name set,
// distributing them round-robin across the provided dev agent list.
func (g *TaskGraph) AssignAgents(devAgents []string) {
	if len(devAgents) == 0 {
		return
	}
	idx := 0
	for i := range g.Tasks {
		if g.Tasks[i].AgentName == "" {
			g.Tasks[i].AgentName = devAgents[idx%len(devAgents)]
			idx++
		}
	}
}
