// Package memory provides the 3-layer memory management system.
// Spec Reference: Section 8 "Agent Memory Architecture"
package memory

import (
	"fmt"
	"strings"

	"github.com/yongjunkang/pylon/internal/config"
	"github.com/yongjunkang/pylon/internal/store"
)

// Manager handles proactive and reactive memory operations.
type Manager struct {
	Store *store.Store
	Cfg   config.MemoryConfig
}

// NewManager creates a new memory manager.
func NewManager(s *store.Store, cfg config.MemoryConfig) *Manager {
	return &Manager{Store: s, Cfg: cfg}
}

// GetProactiveContext searches project memory for context relevant to a task.
// This is injected into CLAUDE.md before agent execution.
// Spec Reference: Section 8 "Proactive Memory Injection"
func (m *Manager) GetProactiveContext(projectID, taskDesc string, maxTokens int) (string, error) {
	if !m.Cfg.ProactiveInjection {
		return "", nil
	}
	if maxTokens <= 0 {
		maxTokens = m.Cfg.ProactiveMaxTokens
	}

	results, err := m.Store.SearchMemory(projectID, taskDesc, 10)
	if err != nil {
		return "", fmt.Errorf("memory search failed: %w", err)
	}

	if len(results) == 0 {
		return "", nil
	}

	var lines []string
	charCount := 0
	charLimit := maxTokens * 4 // rough estimate: 1 token ≈ 4 chars

	for _, r := range results {
		entry := fmt.Sprintf("- [%s] %s: %s", r.Category, r.Key, r.Content)
		if charCount+len(entry) > charLimit {
			break
		}
		lines = append(lines, entry)
		charCount += len(entry)

		// Increment access count for used memories
		m.Store.IncrementAccessCount(r.ID)
	}

	return strings.Join(lines, "\n"), nil
}

// HandleQuery processes a reactive query from an agent.
// Spec Reference: Section 8 "Reactive Memory Access"
func (m *Manager) HandleQuery(projectID, query string, categories []string) ([]store.MemorySearchResult, error) {
	if len(categories) > 0 {
		// Search within specific categories
		var allResults []store.MemorySearchResult
		for _, cat := range categories {
			results, err := m.Store.SearchMemory(projectID, query+" "+cat, 5)
			if err != nil {
				continue
			}
			allResults = append(allResults, results...)
		}
		return allResults, nil
	}

	return m.Store.SearchMemory(projectID, query, 10)
}

// StoreLearnings saves learnings from an agent's result into project memory.
// Spec Reference: Section 8 "Learning Accumulation"
func (m *Manager) StoreLearnings(projectID, taskID, agentName string, learnings []string) error {
	for _, learning := range learnings {
		entry := &store.MemoryEntry{
			ProjectID:  projectID,
			Category:   "learning",
			Key:        fmt.Sprintf("%s/%s", taskID, sanitize(learning[:min(len(learning), 50)])),
			Content:    learning,
			Author:     agentName,
			Confidence: 0.8,
		}
		if err := m.Store.InsertMemory(entry); err != nil {
			return fmt.Errorf("failed to store learning: %w", err)
		}
	}
	return nil
}

func sanitize(s string) string {
	r := strings.NewReplacer(" ", "-", "/", "-", ":", "-")
	return r.Replace(s)
}
