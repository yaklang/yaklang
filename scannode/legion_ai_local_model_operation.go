package scannode

import (
	"context"
	"sync"
)

type aiLocalModelOperationState struct {
	cancel context.CancelFunc
}

type aiLocalModelOperationManager struct {
	mu         sync.Mutex
	operations map[string]aiLocalModelOperationState
}

func newAILocalModelOperationManager() *aiLocalModelOperationManager {
	return &aiLocalModelOperationManager{
		operations: make(map[string]aiLocalModelOperationState),
	}
}

func (m *aiLocalModelOperationManager) Store(operationID string, cancel context.CancelFunc) {
	if m == nil || operationID == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.operations[operationID] = aiLocalModelOperationState{cancel: cancel}
}

func (m *aiLocalModelOperationManager) Cancel(operationID string) bool {
	if m == nil || operationID == "" {
		return false
	}
	m.mu.Lock()
	state, ok := m.operations[operationID]
	m.mu.Unlock()
	if !ok || state.cancel == nil {
		return false
	}
	state.cancel()
	return true
}

func (m *aiLocalModelOperationManager) Remove(operationID string) {
	if m == nil || operationID == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.operations, operationID)
}
