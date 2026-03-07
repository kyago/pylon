// Package protocol defines the MessageEnvelope communication protocol.
// Spec Reference: Section 8 "Hybrid Communication Protocol"
package protocol

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// MessageType represents the type of a protocol message.
type MessageType string

const (
	MsgTaskAssign  MessageType = "task_assign"
	MsgResult      MessageType = "result"
	MsgQuery       MessageType = "query"
	MsgQueryResult MessageType = "query_result"
	MsgBroadcast   MessageType = "broadcast"
	MsgHeartbeat   MessageType = "heartbeat"
)

// Priority represents message priority levels.
type Priority int

const (
	PriorityCritical Priority = 0
	PriorityHigh     Priority = 1
	PriorityNormal   Priority = 2
	PriorityLow      Priority = 3
)

// MessageEnvelope is the standard message format for agent communication.
type MessageEnvelope struct {
	ID        string      `json:"id"`
	Type      MessageType `json:"type"`
	Priority  Priority    `json:"priority"`
	From      string      `json:"from"`
	To        string      `json:"to"`
	ReplyTo   string      `json:"reply_to,omitempty"`
	Subject   string      `json:"subject"`
	Body      any         `json:"body"`
	Context   *MsgContext `json:"context,omitempty"`
	TTL       string      `json:"ttl,omitempty"`
	Trace     []string    `json:"trace,omitempty"`
	Timestamp string      `json:"timestamp"`
}

// MsgContext holds pipeline and task context for a message.
type MsgContext struct {
	TaskID       string   `json:"task_id"`
	PipelineID   string   `json:"pipeline_id,omitempty"`
	ProjectID    string   `json:"project_id,omitempty"`
	ParentTaskID string   `json:"parent_task_id,omitempty"`
	Summary      string   `json:"summary,omitempty"`
	Decisions    []string `json:"decisions,omitempty"`
	Constraints  []string `json:"constraints,omitempty"`
	References   []string `json:"references,omitempty"`
}

// TaskAssignBody is the body payload for task_assign messages.
type TaskAssignBody struct {
	TaskID             string   `json:"task_id"`
	Description        string   `json:"description"`
	Branch             string   `json:"branch,omitempty"`
	AcceptanceCriteria []string `json:"acceptance_criteria,omitempty"`
	ContextFiles       []string `json:"context_files,omitempty"`
}

// ResultBody is the body payload for result messages.
type ResultBody struct {
	TaskID       string   `json:"task_id"`
	Status       string   `json:"status"` // completed, failed, blocked
	FilesChanged []string `json:"files_changed,omitempty"`
	Commits      []string `json:"commits,omitempty"`
	Summary      string   `json:"summary,omitempty"`
	Learnings    []string `json:"learnings,omitempty"`
}

// QueryBody is the body payload for query messages.
type QueryBody struct {
	Query      string   `json:"query"`
	Categories []string `json:"categories,omitempty"`
}

// QueryResultBody is the body payload for query_result messages.
type QueryResultBody struct {
	Results []QueryResultItem `json:"results"`
}

// QueryResultItem is a single result from a memory query.
type QueryResultItem struct {
	Key        string  `json:"key"`
	Content    string  `json:"content"`
	Confidence float64 `json:"confidence"`
}

// NewMessage creates a new MessageEnvelope with a generated ID and timestamp.
func NewMessage(msgType MessageType, from, to string) *MessageEnvelope {
	return &MessageEnvelope{
		ID:        uuid.New().String(),
		Type:      msgType,
		Priority:  PriorityNormal,
		From:      from,
		To:        to,
		Timestamp: time.Now().Format(time.RFC3339),
	}
}

// Marshal serializes the message to JSON bytes.
func (m *MessageEnvelope) Marshal() ([]byte, error) {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal message: %w", err)
	}
	return data, nil
}

// Unmarshal deserializes a message from JSON bytes.
func Unmarshal(data []byte) (*MessageEnvelope, error) {
	msg := &MessageEnvelope{}
	if err := json.Unmarshal(data, msg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal message: %w", err)
	}
	return msg, nil
}
