package scannode

import (
	"context"
	"errors"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

// A company runtime may execute only while its authenticated command transport
// is live. Revocation and expiry fence admissions before cleaning up engines.
// A reconnect authenticates again and must receive fresh dispatch/Bind commands.
func (b *legionJobBridge) invalidateCompanyExecution() {
	if b == nil {
		return
	}
	b.companyCleanupMu.Lock()
	defer b.companyCleanupMu.Unlock()
	b.mu.Lock()
	if b.companyExecutionCancel != nil {
		b.companyExecutionCancel()
	}
	b.mu.Unlock()
	b.stopConsumer()
	b.switchDispatchSession("")
	if b.publisher != nil {
		b.publisher.Close()
	}
	if b.capabilityPublisher != nil {
		b.capabilityPublisher.Close()
	}
	if b.ruleSyncPublisher != nil {
		b.ruleSyncPublisher.Close()
	}
	if b.aiPublisher != nil {
		b.aiPublisher.Close()
	}
	b.aiLocalModelOps.cancelAllForTransportLoss()
	b.aiKnowledgeBaseQueries.cancelAllForTransportLoss()
	b.aiKnowledgeBaseQuestionIndexes.cancelAllForTransportLoss()
	if b.aiRuntime != nil {
		b.aiRuntime.closeForTransportLoss()
	}
	if b.agent != nil && b.agent.runtimeHost != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := b.agent.runtimeHost.stopOwnedContainers(ctx); err != nil {
			log.Errorf("stop company runtime containers after session loss: %v", err)
		}
	}
}

func (m *aiSessionRuntimeManager) closeForTransportLoss() {
	if m == nil {
		return
	}
	m.mu.Lock()
	var handles []aiSessionRuntimeHandle
	var cleanups []func() error
	for id, pending := range m.bindings {
		if pending.cancel != nil {
			pending.cancel()
		}
		delete(m.bindings, id)
		m.recordTerminalTombstoneLocked(id, aiSessionTerminalTombstone{kind: "close", epoch: pending.epoch})
	}
	for id, session := range m.sessions {
		session.mu.Lock()
		session.retired = true
		if session.cancel != nil {
			session.cancel()
		}
		if session.handle != nil {
			handles = append(handles, session.handle)
		}
		workspace, inputs := session.codeWorkspace, session.inputWorkspace
		cleanups = append(cleanups, func() error { return errors.Join(workspace.Cleanup(), inputs.Cleanup()) })
		epoch := session.bindEpoch
		session.mu.Unlock()
		delete(m.sessions, id)
		if previous, ok := m.terminalTombstones[id]; ok && previous.epoch > epoch {
			epoch = previous.epoch
		}
		m.recordTerminalTombstoneLocked(id, aiSessionTerminalTombstone{kind: "close", epoch: epoch})
	}
	m.mu.Unlock()
	for _, handle := range handles {
		handle.Close("authenticated node transport lost")
	}
	for _, cleanup := range cleanups {
		if err := cleanup(); err != nil {
			log.Errorf("cleanup revoked company AI workspace: %v", err)
		}
	}
}

// Use the durable local ownership journal, never a broad Docker list/removal.
// Failed cleanups retain their journal state for the next lifecycle retry.
func (e *runtimeHostExecutor) stopOwnedContainers(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	var failures []error
	for key, record := range e.operations {
		if record.State == "stopped" {
			continue
		}
		container, found, err := e.docker.FindContainer(ctx, key)
		if err == nil && found {
			err = validateRuntimeContainerRecord(container, record, e.agentInstallationID)
		}
		if err == nil && found {
			err = e.docker.StopAndRemove(ctx, container.ID)
		}
		if err != nil {
			failures = append(failures, err)
			continue
		}
		record.State = "stopped"
		record.UpdatedAt = time.Now().UTC()
		e.operations[key] = record
	}
	if err := e.saveOperationJournal(); err != nil {
		failures = append(failures, err)
	}
	return errors.Join(failures...)
}

// Company work survives credential rotation within one valid session, but its
// context is canceled by revocation, expiry, or an unexpected transport loss.
func (b *legionJobBridge) companyExecutionContext(fallback context.Context) context.Context {
	if !b.companyBound.Load() {
		return fallback
	}
	b.mu.Lock()
	ctx := b.companyExecutionCtx
	b.mu.Unlock()
	if ctx != nil {
		return ctx
	}
	ctx, cancel := context.WithCancel(fallback)
	cancel()
	return ctx
}

func (m *aiLocalModelOperationManager) cancelAllForTransportLoss() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, state := range m.operations {
		if state.cancel != nil {
			state.cancel()
		}
		delete(m.operations, id)
	}
}

func (m *aiKnowledgeBaseQueryManager) cancelAllForTransportLoss() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, state := range m.queries {
		if state.cancel != nil {
			state.cancel()
		}
		delete(m.queries, id)
	}
}
