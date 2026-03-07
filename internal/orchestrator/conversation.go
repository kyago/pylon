package orchestrator

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// ConversationManager manages conversation thread files.
// Spec Reference: Section 9 "Conversation History"
type ConversationManager struct {
	BaseDir string // .pylon/conversations/
}

// ConversationMeta holds metadata for a conversation.
type ConversationMeta struct {
	Status    string   `yaml:"status"`
	StartedAt string   `yaml:"started_at"`
	Projects  []string `yaml:"projects,omitempty"`
	TaskID    string   `yaml:"task_id,omitempty"`
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
		Status:    "active",
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

	metaPath := filepath.Join(dir, "meta.yml")
	if err := os.WriteFile(metaPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write meta: %w", err)
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
