package scannode

import (
	"context"
	"sync"
)

type aiKnowledgeBaseQueryState struct {
	cancel context.CancelFunc
}

type aiKnowledgeBaseQueryManager struct {
	mu      sync.Mutex
	queries map[string]aiKnowledgeBaseQueryState
}

func newAIKnowledgeBaseQueryManager() *aiKnowledgeBaseQueryManager {
	return &aiKnowledgeBaseQueryManager{
		queries: make(map[string]aiKnowledgeBaseQueryState),
	}
}

func (m *aiKnowledgeBaseQueryManager) Store(commandID string, cancel context.CancelFunc) {
	if m == nil || commandID == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queries[commandID] = aiKnowledgeBaseQueryState{cancel: cancel}
}

func (m *aiKnowledgeBaseQueryManager) Cancel(commandID string) bool {
	if m == nil || commandID == "" {
		return false
	}
	m.mu.Lock()
	state, ok := m.queries[commandID]
	m.mu.Unlock()
	if !ok || state.cancel == nil {
		return false
	}
	state.cancel()
	return true
}

func (m *aiKnowledgeBaseQueryManager) Remove(commandID string) {
	if m == nil || commandID == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.queries, commandID)
}
