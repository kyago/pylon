// Package memory provides the 3-layer memory management system.
// Spec Reference: Section 8 "Agent Memory Architecture"
package memory

import (
	"fmt"
	"strings"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/store"
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

// StoreLearnings saves learnings from an agent's result into project memory.
// Spec Reference: Section 8 "Learning Accumulation"
func (m *Manager) StoreLearnings(projectID, taskID, agentName string, learnings []string) error {
	for _, learning := range learnings {
		// 바이트가 아닌 룬 단위로 잘라 멀티바이트 문자가 깨지지 않게 한다.
		keyRunes := []rune(learning)
		if len(keyRunes) > 50 {
			keyRunes = keyRunes[:50]
		}
		entry := &store.MemoryEntry{
			ProjectID:  projectID,
			Category:   "learning",
			Key:        fmt.Sprintf("%s/%s", taskID, sanitize(string(keyRunes))),
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
