package orchestrator

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kyago/pylon/internal/store"
)

// Conversation status constants.
const (
	ConvStatusActive    = "active"
	ConvStatusCompleted = "completed"
	ConvStatusCancelled = "cancelled"
)

// ConversationManager manages conversation thread files and metadata.
// Metadata is stored in SQLite via Store; thread.md files remain on disk.
type ConversationManager struct {
	BaseDir string // .pylon/conversations/
	Store   *store.Store
}

// ClarityScores holds per-dimension clarity scores (0.0–1.0).
type ClarityScores struct {
	Goal        float64 `json:"goal"`
	Constraints float64 `json:"constraints"`
	Criteria    float64 `json:"criteria"`
	Context     float64 `json:"context,omitempty"`
}

// ConversationMeta holds metadata for a conversation.
type ConversationMeta struct {
	Status         string
	StartedAt      string
	CompletedAt    string
	SessionID      string
	Projects       []string
	TaskID         string
	AmbiguityScore float64
	ClarityScores  *ClarityScores
}

// Conversation represents a single conversation thread.
type Conversation struct {
	ID   string
	Dir  string
	Meta ConversationMeta
}

// NewConversationManager creates a new manager.
// Store must be non-nil; all metadata operations require SQLite.
func NewConversationManager(baseDir string, s *store.Store) *ConversationManager {
	if s == nil {
		panic("ConversationManager requires a non-nil Store")
	}
	return &ConversationManager{BaseDir: baseDir, Store: s}
}

// Create initializes a new conversation with metadata in SQLite and thread.md on disk.
func (c *ConversationManager) Create(id, title string) (*Conversation, error) {
	dir := filepath.Join(c.BaseDir, id)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create conversation dir: %w", err)
	}

	now := time.Now()
	meta := ConversationMeta{
		Status:    ConvStatusActive,
		StartedAt: now.Format(time.RFC3339),
	}

	// Store metadata in SQLite
	if err := c.Store.UpsertConversation(&store.ConversationRecord{
		ID:        id,
		Title:     title,
		Status:    ConvStatusActive,
		StartedAt: now,
		UpdatedAt: now,
	}); err != nil {
		return nil, fmt.Errorf("failed to save conversation to store: %w", err)
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

// SaveMeta updates the conversation metadata in SQLite.
// The existing title is preserved (ConversationMeta does not carry a title).
func (c *ConversationManager) SaveMeta(id string, meta ConversationMeta) error {
	existing, err := c.Store.GetConversation(id)
	if err != nil {
		return fmt.Errorf("failed to load existing conversation for SaveMeta: %w", err)
	}
	title := ""
	if existing != nil {
		title = existing.Title
	}
	rec := metaToRecord(id, title, meta)
	return c.Store.UpsertConversation(rec)
}

// Complete marks a conversation as completed.
func (c *ConversationManager) Complete(id string) error {
	now := time.Now()
	return c.Store.UpdateConversationStatus(id, ConvStatusCompleted, &now)
}

// Load reads an existing conversation from SQLite.
func (c *ConversationManager) Load(id string) (*Conversation, error) {
	rec, err := c.Store.GetConversation(id)
	if err != nil {
		return nil, fmt.Errorf("failed to load conversation: %w", err)
	}
	if rec == nil {
		return nil, fmt.Errorf("conversation %q not found", id)
	}

	dir := filepath.Join(c.BaseDir, id)
	meta := recordToMeta(rec)

	return &Conversation{ID: id, Dir: dir, Meta: meta}, nil
}

// List returns conversations filtered by status.
// If status is empty, all conversations are returned.
func (c *ConversationManager) List(status string) ([]Conversation, error) {
	recs, err := c.Store.ListConversations(status)
	if err != nil {
		return nil, err
	}

	var result []Conversation
	for _, rec := range recs {
		dir := filepath.Join(c.BaseDir, rec.ID)
		result = append(result, Conversation{
			ID:   rec.ID,
			Dir:  dir,
			Meta: recordToMeta(&rec),
		})
	}
	return result, nil
}

// --- Conversion helpers ---

func metaToRecord(id, title string, meta ConversationMeta) *store.ConversationRecord {
	rec := &store.ConversationRecord{
		ID:             id,
		Title:          title,
		Status:         meta.Status,
		SessionID:      meta.SessionID,
		TaskID:         meta.TaskID,
		AmbiguityScore: meta.AmbiguityScore,
		UpdatedAt:      time.Now(),
	}

	if meta.StartedAt != "" {
		if t, err := time.Parse(time.RFC3339, meta.StartedAt); err == nil {
			rec.StartedAt = t
		}
	}
	if meta.CompletedAt != "" {
		if t, err := time.Parse(time.RFC3339, meta.CompletedAt); err == nil {
			rec.CompletedAt = &t
		}
	}
	if len(meta.Projects) > 0 {
		rec.Projects = strings.Join(meta.Projects, ",")
	}
	if meta.ClarityScores != nil {
		rec.ClarityScores = marshalClarityScores(meta.ClarityScores)
	}

	return rec
}

func recordToMeta(rec *store.ConversationRecord) ConversationMeta {
	meta := ConversationMeta{
		Status:         rec.Status,
		StartedAt:      rec.StartedAt.Format(time.RFC3339),
		SessionID:      rec.SessionID,
		TaskID:         rec.TaskID,
		AmbiguityScore: rec.AmbiguityScore,
	}
	if rec.CompletedAt != nil {
		meta.CompletedAt = rec.CompletedAt.Format(time.RFC3339)
	}
	if rec.Projects != "" {
		meta.Projects = strings.Split(rec.Projects, ",")
	}
	if rec.ClarityScores != "" {
		meta.ClarityScores = unmarshalClarityScores(rec.ClarityScores)
	}
	return meta
}

func marshalClarityScores(scores *ClarityScores) string {
	data, err := json.Marshal(scores)
	if err != nil {
		return ""
	}
	return string(data)
}

func unmarshalClarityScores(s string) *ClarityScores {
	if s == "" {
		return nil
	}
	var scores ClarityScores
	if err := json.Unmarshal([]byte(s), &scores); err != nil {
		return nil
	}
	return &scores
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
