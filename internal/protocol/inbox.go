package protocol

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteTask writes a task assignment message to an agent's inbox.
// Path: {inboxDir}/{agentName}/{taskID}.task.json
// Uses atomic write (write to .tmp then rename) to prevent partial reads.
func WriteTask(inboxDir, agentName string, msg *MessageEnvelope) error {
	agentDir := filepath.Join(inboxDir, agentName)
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		return fmt.Errorf("failed to create inbox dir: %w", err)
	}

	// Extract task_id from body
	taskID := extractTaskID(msg)
	if taskID == "" {
		taskID = msg.ID
	}

	data, err := msg.Marshal()
	if err != nil {
		return err
	}

	targetPath := filepath.Join(agentDir, taskID+".task.json")
	return writeAtomically(targetPath, data)
}

// extractTaskID attempts to get the task_id from the message body or context.
func extractTaskID(msg *MessageEnvelope) string {
	if msg.Context != nil && msg.Context.TaskID != "" {
		return msg.Context.TaskID
	}
	// Try to extract from body if it's a map
	if bodyMap, ok := msg.Body.(map[string]any); ok {
		if taskID, ok := bodyMap["task_id"].(string); ok {
			return taskID
		}
	}
	return ""
}

// writeAtomically writes data to a file using the tmp+rename pattern.
// This ensures POSIX rename(2) atomicity — readers never see partial writes.
func writeAtomically(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("failed to write tmp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp) // cleanup on failure
		return fmt.Errorf("failed to rename tmp to final: %w", err)
	}
	return nil
}
