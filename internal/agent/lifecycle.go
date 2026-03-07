package agent

import (
	"fmt"
	"time"
)

// State represents the lifecycle state of an agent.
type State string

const (
	StateIdle      State = "idle"
	StateStarting  State = "starting"
	StateRunning   State = "running"
	StateCompleted State = "completed"
	StateFailed    State = "failed"
	StateCancelled State = "cancelled"
)

// validLifecycleTransitions defines which state transitions are allowed.
var validLifecycleTransitions = map[State][]State{
	StateIdle:     {StateStarting},
	StateStarting: {StateRunning, StateFailed},
	StateRunning:  {StateCompleted, StateFailed, StateCancelled},
}

// Lifecycle tracks the execution state of an agent.
type Lifecycle struct {
	AgentName   string
	State       State
	TaskID      string
	TmuxSession string
	StartedAt   time.Time
	Timeout     time.Duration
}

// NewLifecycle creates a new agent lifecycle in idle state.
func NewLifecycle(agentName, taskID, tmuxSession string, timeout time.Duration) *Lifecycle {
	return &Lifecycle{
		AgentName:   agentName,
		State:       StateIdle,
		TaskID:      taskID,
		TmuxSession: tmuxSession,
		Timeout:     timeout,
	}
}

// Transition moves the lifecycle to a new state if the transition is valid.
func (l *Lifecycle) Transition(to State) error {
	allowed, ok := validLifecycleTransitions[l.State]
	if !ok {
		return fmt.Errorf("no transitions from terminal state %q", l.State)
	}

	for _, s := range allowed {
		if s == to {
			if to == StateRunning {
				l.StartedAt = time.Now()
			}
			l.State = to
			return nil
		}
	}

	return fmt.Errorf("invalid lifecycle transition: %s → %s", l.State, to)
}

// CheckTimeout returns true if the agent has exceeded its timeout.
func (l *Lifecycle) CheckTimeout() bool {
	if l.State != StateRunning || l.Timeout <= 0 {
		return false
	}
	return time.Since(l.StartedAt) > l.Timeout
}

// IsTerminal returns true if the agent is in a terminal state.
func (l *Lifecycle) IsTerminal() bool {
	return l.State == StateCompleted || l.State == StateFailed || l.State == StateCancelled
}
