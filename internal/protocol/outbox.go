package protocol

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ReadResult reads a result message from an agent's outbox file.
// It handles both the full MessageEnvelope format and the flat ResultBody format
// that agents write (simpler JSON without the envelope wrapper).
func ReadResult(path string) (*MessageEnvelope, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read result file: %w", err)
	}

	// Try full envelope first.
	// Require env.Type to be non-empty: valid JSON without a "type" field
	// (e.g. flat agent output) should fall through to flat JSON parsing below,
	// which wraps it in a proper envelope with MsgResult type.
	env, envErr := Unmarshal(data)
	if envErr == nil && env.Type != "" {
		return env, nil
	}

	// Try flat result body — agents write this simpler format
	var raw map[string]any
	if jsonErr := json.Unmarshal(data, &raw); jsonErr != nil {
		// Return original envelope error if both fail
		if envErr != nil {
			return nil, fmt.Errorf("failed to parse result (envelope: %w, flat: %v)", envErr, jsonErr)
		}
		return nil, fmt.Errorf("failed to parse result as JSON: %w", jsonErr)
	}

	// Wrap flat result in a MessageEnvelope
	wrapped := &MessageEnvelope{
		Type: MsgResult,
		Body: raw,
	}

	// Extract context from flat fields
	if taskID, ok := raw["task_id"].(string); ok {
		wrapped.Context = &MsgContext{TaskID: taskID}
	}

	return wrapped, nil
}

// WriteResult writes a result message to an agent's outbox.
// Path: {outboxDir}/{agentName}/{taskID}.result.json
func WriteResult(outboxDir, agentName string, msg *MessageEnvelope) error {
	agentDir := filepath.Join(outboxDir, agentName)
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		return fmt.Errorf("failed to create outbox dir: %w", err)
	}

	taskID := extractTaskID(msg)
	if taskID == "" {
		taskID = msg.ID
	}

	data, err := msg.Marshal()
	if err != nil {
		return err
	}

	targetPath := filepath.Join(agentDir, taskID+".result.json")
	return writeAtomically(targetPath, data)
}

// ScanOutbox scans an outbox directory for result files.
func ScanOutbox(outboxDir string) ([]string, error) {
	var results []string

	entries, err := os.ReadDir(outboxDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read outbox dir: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		agentDir := filepath.Join(outboxDir, entry.Name())
		files, err := os.ReadDir(agentDir)
		if err != nil {
			continue
		}
		for _, f := range files {
			if strings.HasSuffix(f.Name(), ".result.json") {
				results = append(results, filepath.Join(agentDir, f.Name()))
			}
		}
	}

	return results, nil
}
