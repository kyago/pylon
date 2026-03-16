package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/store"
)

// Orchestrator coordinates the pylon pipeline execution.
// Spec Reference: Section 8 "Orchestrator Core"
type Orchestrator struct {
	Config     *config.Config
	Store      *store.Store
	WorkDir    string // workspace root
	Pipeline   *Pipeline
	pipelineID string // optional: used by Recover() to target a specific pipeline
}

// NewOrchestrator creates a new orchestrator instance.
func NewOrchestrator(cfg *config.Config, s *store.Store, workDir string) *Orchestrator {
	return &Orchestrator{
		Config:  cfg,
		Store:   s,
		WorkDir: workDir,
	}
}

// SetPipelineID sets the pipeline ID for targeted recovery.
func (o *Orchestrator) SetPipelineID(id string) {
	o.pipelineID = id
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

// savePipelineState persists the pipeline state to SQLite (single source of truth).
func (o *Orchestrator) savePipelineState() error {
	data, err := o.Pipeline.Snapshot()
	if err != nil {
		return err
	}

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

	return nil
}

// UnprocessedResult represents an outbox result that was not processed before crash.
type UnprocessedResult struct {
	AgentName string
	FilePath  string
	TaskID    string
}

// Recover restores pipeline state from SQLite after orchestrator crash.
// Returns any unprocessed outbox results found during recovery.
// Spec Reference: Section 8 "SPOF Recovery"
func (o *Orchestrator) Recover() ([]UnprocessedResult, error) {
	if o.Store == nil {
		return nil, nil // no store available
	}

	// If a specific pipeline ID is known, recover it directly
	if o.pipelineID != "" {
		rec, err := o.Store.GetPipeline(o.pipelineID)
		if err != nil {
			return nil, fmt.Errorf("failed to get pipeline from store: %w", err)
		}
		if rec == nil {
			return nil, nil // not found, fresh start
		}
		pipeline, err := LoadPipeline([]byte(rec.StateJSON))
		if err != nil {
			return nil, fmt.Errorf("failed to parse pipeline state: %w", err)
		}
		if pipeline.IsTerminal() {
			return nil, nil // already done
		}
		o.Pipeline = pipeline
	} else {
		// Recover the most recent active pipeline
		actives, err := o.Store.GetActivePipelines()
		if err != nil {
			return nil, fmt.Errorf("failed to get active pipelines: %w", err)
		}
		if len(actives) == 0 {
			return nil, nil // no active pipeline
		}
		pipeline, err := LoadPipeline([]byte(actives[0].StateJSON))
		if err != nil {
			return nil, fmt.Errorf("failed to parse pipeline state: %w", err)
		}
		if pipeline.IsTerminal() {
			return nil, nil
		}
		o.Pipeline = pipeline
	}

	// Scan for unprocessed outbox results
	var unprocessed []UnprocessedResult
	outboxDir := filepath.Join(o.WorkDir, ".pylon", "runtime", "outbox")
	entries, err := os.ReadDir(outboxDir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				agentName := entry.Name()
				agentDir := filepath.Join(outboxDir, agentName)
				files, _ := os.ReadDir(agentDir)
				for _, f := range files {
					if strings.HasSuffix(f.Name(), ".result.json") {
						taskID := strings.TrimSuffix(f.Name(), ".result.json")
						processed, chkErr := o.Store.IsResultProcessed(agentName, taskID)
						if chkErr != nil {
							fmt.Printf("[recovery] failed to check result status for %s/%s: %v\n", agentName, f.Name(), chkErr)
							continue
						}
						if !processed {
							fmt.Printf("[recovery] unprocessed result: %s/%s\n", agentName, f.Name())
							unprocessed = append(unprocessed, UnprocessedResult{
								AgentName: agentName,
								FilePath:  filepath.Join(agentDir, f.Name()),
								TaskID:    taskID,
							})
						}
					}
				}
			}
		}
	}

	return unprocessed, o.savePipelineState()
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
