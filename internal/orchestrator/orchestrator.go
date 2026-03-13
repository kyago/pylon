package orchestrator

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/store"
)

// Orchestrator coordinates the pylon pipeline execution.
// Spec Reference: Section 8 "Orchestrator Core"
type Orchestrator struct {
	Config   *config.Config
	Store    *store.Store
	WorkDir  string // workspace root
	Pipeline *Pipeline
}

// NewOrchestrator creates a new orchestrator instance.
func NewOrchestrator(cfg *config.Config, s *store.Store, workDir string) *Orchestrator {
	return &Orchestrator{
		Config:  cfg,
		Store:   s,
		WorkDir: workDir,
	}
}

// StartPipeline creates a new pipeline and persists its initial state.
func (o *Orchestrator) StartPipeline(id string) error {
	o.Pipeline = NewPipeline(id, o.Config.Runtime.MaxAttempts)
	return o.savePipelineState()
}

// TransitionTo advances the pipeline to the next stage.
func (o *Orchestrator) TransitionTo(stage Stage) error {
	if o.Pipeline == nil {
		return fmt.Errorf("no active pipeline")
	}

	if err := o.Pipeline.Transition(stage); err != nil {
		return err
	}

	return o.savePipelineState()
}

// SaveState persists the current pipeline state (public API).
func (o *Orchestrator) SaveState() error {
	return o.savePipelineState()
}

// ForceStage sets the pipeline stage directly, bypassing transition validation.
// Use only for rollback scenarios where normal transitions are not possible.
// Records the forced transition in History for audit/debugging purposes.
func (o *Orchestrator) ForceStage(stage Stage) error {
	if o.Pipeline == nil {
		return fmt.Errorf("no active pipeline")
	}
	o.Pipeline.History = append(o.Pipeline.History, StageTransition{
		From:        o.Pipeline.CurrentStage,
		To:          stage,
		CompletedAt: time.Now(),
	})
	o.Pipeline.CurrentStage = stage
	return o.savePipelineState()
}

// savePipelineState persists the pipeline to both SQLite and state.json.
func (o *Orchestrator) savePipelineState() error {
	data, err := o.Pipeline.Snapshot()
	if err != nil {
		return err
	}

	// Save to SQLite
	if o.Store != nil {
		if err := o.Store.UpsertPipeline(&store.PipelineRecord{
			PipelineID: o.Pipeline.ID,
			Stage:      string(o.Pipeline.CurrentStage),
			StateJSON:  string(data),
			UpdatedAt:  time.Now(),
		}); err != nil {
			return fmt.Errorf("failed to persist pipeline state: %w", err)
		}
	}

	// Save to state.json (SPOF recovery)
	stateDir := filepath.Join(o.WorkDir, ".pylon", "runtime")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return fmt.Errorf("failed to create state dir: %w", err)
	}
	statePath := filepath.Join(stateDir, "state.json")

	tmp := statePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, statePath)
}

// Recover restores pipeline state after orchestrator crash.
// Spec Reference: Section 8 "SPOF Recovery"
func (o *Orchestrator) Recover() error {
	statePath := filepath.Join(o.WorkDir, ".pylon", "runtime", "state.json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil // no state to recover
		}
		return fmt.Errorf("failed to read state.json: %w", err)
	}

	pipeline, err := LoadPipeline(data)
	if err != nil {
		return fmt.Errorf("failed to parse state.json: %w", err)
	}

	if pipeline.IsTerminal() {
		return nil // already done
	}

	o.Pipeline = pipeline

	// Scan for unprocessed outbox results
	outboxDir := filepath.Join(o.WorkDir, ".pylon", "runtime", "outbox")
	entries, err := os.ReadDir(outboxDir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				agentDir := filepath.Join(outboxDir, entry.Name())
				files, _ := os.ReadDir(agentDir)
				for _, f := range files {
					if filepath.Ext(f.Name()) == ".json" && !isProcessed(agentDir, f.Name()) {
						// Mark as needing processing
						fmt.Printf("[recovery] unprocessed result: %s/%s\n", entry.Name(), f.Name())
					}
				}
			}
		}
	}

	return o.savePipelineState()
}

// isProcessed checks whether a result file has been marked as processed.
// Convention: processed files have a companion ".done" marker file.
func isProcessed(dir, filename string) bool {
	donePath := filepath.Join(dir, filename+".done")
	_, err := os.Stat(donePath)
	return err == nil
}

// markProcessed creates a ".done" marker for a processed result file.
func markProcessed(dir, filename string) error {
	donePath := filepath.Join(dir, filename+".done")
	return os.WriteFile(donePath, []byte{}, 0644)
}

// GetStatus returns a summary of the current orchestrator state.
func (o *Orchestrator) GetStatus() map[string]any {
	status := map[string]any{
		"workspace": o.WorkDir,
	}

	if o.Pipeline != nil {
		status["pipeline_id"] = o.Pipeline.ID
		status["stage"] = o.Pipeline.CurrentStage
		status["agents"] = o.Pipeline.Agents
		status["created_at"] = o.Pipeline.CreatedAt
	} else {
		status["pipeline"] = "none"
	}

	return status
}

// GetStatusJSON returns the status as pretty-printed JSON.
func (o *Orchestrator) GetStatusJSON() (string, error) {
	data, err := json.MarshalIndent(o.GetStatus(), "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
