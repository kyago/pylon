package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kyago/pylon/internal/protocol"
	"github.com/kyago/pylon/internal/store"
)

// WatchResult holds a parsed outbox result with its file path.
type WatchResult struct {
	AgentName string
	FilePath  string
	Envelope  *protocol.MessageEnvelope
}

// OutboxWatcher polls the outbox directory for agent results.
type OutboxWatcher struct {
	OutboxDir    string
	PollInterval time.Duration
	Store        *store.Store
}

// NewOutboxWatcher creates a new watcher with the given outbox directory, poll interval, and store.
// If store is nil, falls back to .done marker file checks.
func NewOutboxWatcher(outboxDir string, interval time.Duration, s *store.Store) *OutboxWatcher {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	return &OutboxWatcher{
		OutboxDir:    outboxDir,
		PollInterval: interval,
		Store:        s,
	}
}

// PollOnce scans for unprocessed result files and returns them.
func (w *OutboxWatcher) PollOnce() ([]WatchResult, error) {
	var results []WatchResult

	entries, err := os.ReadDir(w.OutboxDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read outbox dir: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		agentName := entry.Name()
		agentDir := filepath.Join(w.OutboxDir, agentName)

		files, err := os.ReadDir(agentDir)
		if err != nil {
			continue
		}

		for _, f := range files {
			if !strings.HasSuffix(f.Name(), ".result.json") {
				continue
			}
			if w.isProcessed(agentName, agentDir, f.Name()) {
				continue
			}

			filePath := filepath.Join(agentDir, f.Name())
			env, err := protocol.ReadResult(filePath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "⚠ failed to parse outbox result %s: %v\n", filePath, err)
				continue
			}

			results = append(results, WatchResult{
				AgentName: agentName,
				FilePath:  filePath,
				Envelope:  env,
			})
		}
	}

	return results, nil
}

// WaitForResults blocks until results from all expected agents are found or context is cancelled.
// NOTE: Phase 0에서는 순차 실행 후 PollOnce()를 사용. 비동기 병렬 에이전트 실행 시 활용 예정.
func (w *OutboxWatcher) WaitForResults(ctx context.Context, expectedAgents []string) ([]WatchResult, error) {
	if len(expectedAgents) == 0 {
		return nil, nil
	}

	expected := make(map[string]bool)
	for _, name := range expectedAgents {
		expected[name] = true
	}

	var collected []WatchResult
	found := make(map[string]bool)

	ticker := time.NewTicker(w.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return collected, ctx.Err()
		case <-ticker.C:
			results, err := w.PollOnce()
			if err != nil {
				return collected, err
			}

			for _, r := range results {
				if expected[r.AgentName] && !found[r.AgentName] {
					collected = append(collected, r)
					found[r.AgentName] = true
				}
			}

			if len(found) >= len(expected) {
				return collected, nil
			}
		}
	}
}

// isProcessed checks whether a result file has already been processed.
// Uses Store.IsResultProcessed if Store is available, otherwise falls back to .done marker files.
func (w *OutboxWatcher) isProcessed(agentName, agentDir, filename string) bool {
	if w.Store != nil {
		taskID := strings.TrimSuffix(filename, ".result.json")
		processed, err := w.Store.IsResultProcessed(agentName, taskID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "⚠ failed to check processed status via store: %v\n", err)
			return false
		}
		return processed
	}
	// Fallback: .done marker file
	donePath := filepath.Join(agentDir, filename+".done")
	_, err := os.Stat(donePath)
	return err == nil
}
