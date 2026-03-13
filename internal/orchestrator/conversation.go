package orchestrator

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Conversation status constants.
const (
	ConvStatusActive    = "active"
	ConvStatusCompleted = "completed"
	ConvStatusCancelled = "cancelled"
)

// ConversationManager manages conversation thread files.
// Spec Reference: Section 9 "Conversation History"
type ConversationManager struct {
	BaseDir string // .pylon/conversations/
}

// ClarityScores holds per-dimension clarity scores (0.0–1.0).
type ClarityScores struct {
	Goal        float64 `yaml:"goal"`
	Constraints float64 `yaml:"constraints"`
	Criteria    float64 `yaml:"criteria"`
	Context     float64 `yaml:"context,omitempty"`
}

// ConversationMeta holds metadata for a conversation.
type ConversationMeta struct {
	Status         string         `yaml:"status"`
	StartedAt      string         `yaml:"started_at"`
	CompletedAt    string         `yaml:"completed_at,omitempty"`
	SessionID      string         `yaml:"session_id,omitempty"`
	Projects       []string       `yaml:"projects,omitempty"`
	TaskID         string         `yaml:"task_id,omitempty"`
	AmbiguityScore float64        `yaml:"ambiguity_score"`
	ClarityScores  *ClarityScores `yaml:"clarity_scores,omitempty"`
}

// Conversation represents a single conversation thread.
type Conversation struct {
	ID   string
	Dir  string
	Meta ConversationMeta
}

// NewConversationManager creates a new manager.
func NewConversationManager(baseDir string) *ConversationManager {
	return &ConversationManager{BaseDir: baseDir}
}

// Create initializes a new conversation directory with metadata.
func (c *ConversationManager) Create(id, title string) (*Conversation, error) {
	dir := filepath.Join(c.BaseDir, id)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create conversation dir: %w", err)
	}

	meta := ConversationMeta{
		Status:    ConvStatusActive,
		StartedAt: time.Now().Format(time.RFC3339),
	}

	if err := c.saveMeta(dir, meta); err != nil {
		return nil, err
	}

	// Create initial thread.md
	threadPath := filepath.Join(dir, "thread.md")
	header := fmt.Sprintf("# 대화: %s\n\n", title)
	if err := os.WriteFile(threadPath, []byte(header), 0644); err != nil {
		return nil, fmt.Errorf("failed to create thread.md: %w", err)
	}

	return &Conversation{ID: id, Dir: dir, Meta: meta}, nil
}

// AppendMessage adds a message to the conversation thread.
func (c *ConversationManager) AppendMessage(id, role, content string) error {
	threadPath := filepath.Join(c.BaseDir, id, "thread.md")

	f, err := os.OpenFile(threadPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("failed to open thread: %w", err)
	}
	defer f.Close()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	entry := fmt.Sprintf("\n## [%s] %s\n%s\n", timestamp, role, content)

	if _, err := f.WriteString(entry); err != nil {
		return fmt.Errorf("failed to append message: %w", err)
	}

	return nil
}

// SaveMeta updates the conversation metadata.
func (c *ConversationManager) SaveMeta(id string, meta ConversationMeta) error {
	dir := filepath.Join(c.BaseDir, id)
	return c.saveMeta(dir, meta)
}

func (c *ConversationManager) saveMeta(dir string, meta ConversationMeta) error {
	data, err := yaml.Marshal(meta)
	if err != nil {
		return fmt.Errorf("failed to marshal meta: %w", err)
	}

	// Atomic write: write to temp file then rename
	metaPath := filepath.Join(dir, "meta.yml")
	tmpPath := metaPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write meta: %w", err)
	}
	if err := os.Rename(tmpPath, metaPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename meta: %w", err)
	}

	return nil
}

// Load reads an existing conversation.
func (c *ConversationManager) Load(id string) (*Conversation, error) {
	dir := filepath.Join(c.BaseDir, id)

	metaPath := filepath.Join(dir, "meta.yml")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read meta: %w", err)
	}

	var meta ConversationMeta
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("failed to parse meta: %w", err)
	}

	return &Conversation{ID: id, Dir: dir, Meta: meta}, nil
}

// ComputeAmbiguity calculates the ambiguity score from clarity dimensions.
// Greenfield (brownfield=false): 1 - (goal*0.40 + constraints*0.30 + criteria*0.30)
// Brownfield (brownfield=true):  1 - (goal*0.35 + constraints*0.25 + criteria*0.25 + context*0.15)
// Each dimension is clamped to [0, 1]. Result is clamped to [0, 1].
func ComputeAmbiguity(scores ClarityScores, brownfield bool) float64 {
	clamp := func(v float64) float64 {
		return math.Max(0, math.Min(1, v))
	}

	var clarity float64
	if brownfield {
		clarity = clamp(scores.Goal)*0.35 +
			clamp(scores.Constraints)*0.25 +
			clamp(scores.Criteria)*0.25 +
			clamp(scores.Context)*0.15
	} else {
		clarity = clamp(scores.Goal)*0.40 +
			clamp(scores.Constraints)*0.30 +
			clamp(scores.Criteria)*0.30
	}

	return clamp(1 - clarity)
}

// IsReadyForExecution returns true if the conversation's ambiguity score
// is at or below the given threshold, meaning requirements are clear enough.
// Returns false if ClarityScores has never been computed (nil).
func (conv *Conversation) IsReadyForExecution(threshold float64) bool {
	if conv.Meta.ClarityScores == nil {
		return false
	}
	return conv.Meta.AmbiguityScore <= threshold
}
