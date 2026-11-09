package reactloops

import (
	"context"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/asyncexecmanager"
)

const AsyncExecutionSyncNodeID = "async_execution"

// AsyncExecutionSnapshot is shared by push events and explicit sync queries.
// RunningCount includes pending, running and stopping executions because each
// one still owns an active execution slot until its cleanup reaches a terminal
// state.
type AsyncExecutionSnapshot struct {
	Revision      uint64                     `json:"revision"`
	SessionID     string                     `json:"session_id,omitempty"`
	CurrentTaskID string                     `json:"current_task_id,omitempty"`
	RunningCount  int                        `json:"running_count"`
	UpdatedAt     int64                      `json:"updated_at"`
	Executions    []asyncexecmanager.Summary `json:"executions"`
	Execution     *asyncexecmanager.Summary  `json:"execution,omitempty"`
	Error         string                     `json:"error,omitempty"`
}

// BuildAsyncExecutionSnapshot builds the shared metadata-only wire payload.
func BuildAsyncExecutionSnapshot(overview asyncexecmanager.Overview, sessionID, currentTaskID string) AsyncExecutionSnapshot {
	executions := overview.Executions
	if executions == nil {
		executions = make([]asyncexecmanager.Summary, 0)
	}
	return AsyncExecutionSnapshot{
		Revision:      overview.Revision,
		SessionID:     sessionID,
		CurrentTaskID: currentTaskID,
		RunningCount:  overview.RunningCount(),
		UpdatedAt:     time.Now().UnixMilli(),
		Executions:    executions,
	}
}

// AsyncExecutionSnapshot returns a metadata-only view of this loop's manager.
// Session-scoped invoker providers are delegated to so task-local snapshots do
// not hide work started in an earlier turn.
func (r *ReActLoop) AsyncExecutionSnapshot() AsyncExecutionSnapshot {
	if r == nil {
		return BuildAsyncExecutionSnapshot(asyncexecmanager.Overview{}, "", "")
	}
	if provider, ok := r.invoker.(interface {
		AsyncExecutionSnapshot() AsyncExecutionSnapshot
	}); ok {
		return provider.AsyncExecutionSnapshot()
	}
	manager := r.PeekAsyncExecutionManager()
	if manager == nil {
		return BuildAsyncExecutionSnapshot(asyncexecmanager.Overview{}, r.persistentSessionID(), r.currentTaskID())
	}
	return BuildAsyncExecutionSnapshot(manager.Overview(), r.persistentSessionID(), r.currentTaskID())
}

func (r *ReActLoop) persistentSessionID() string {
	if r == nil || r.config == nil {
		return ""
	}
	if provider, ok := r.config.(interface{ GetPersistentSessionID() string }); ok {
		return provider.GetPersistentSessionID()
	}
	return ""
}

func (r *ReActLoop) currentTaskID() string {
	if r == nil {
		return ""
	}
	if task := r.GetCurrentTask(); task != nil {
		return task.GetId()
	}
	return ""
}

// PeekAsyncExecutionManager returns the existing manager without changing lazy
// manager creation semantics or task lifetime.
func (r *ReActLoop) PeekAsyncExecutionManager() *asyncexecmanager.Manager {
	if r == nil {
		return nil
	}
	if provider, ok := r.invoker.(interface {
		PeekAsyncExecutionManager() *asyncexecmanager.Manager
	}); ok {
		return provider.PeekAsyncExecutionManager()
	}
	r.onReleaseMutex.Lock()
	defer r.onReleaseMutex.Unlock()
	return r.asyncExecutionManager
}

// StopAsyncExecution requests cancellation of one execution owned by this
// loop or its session-scoped invoker. A stopping state is not terminal.
func (r *ReActLoop) StopAsyncExecution(executeID string) (asyncexecmanager.Summary, error) {
	manager := r.PeekAsyncExecutionManager()
	if manager == nil {
		return asyncexecmanager.Summary{}, asyncexecmanager.ErrNotFound
	}
	return manager.Stop(executeID)
}

// WatchAsyncExecutionManager publishes every coalesced manager revision using
// the snapshot-before-wait sequence required to avoid lost lifecycle updates.
func WatchAsyncExecutionManager(
	ctx context.Context,
	manager *asyncexecmanager.Manager,
	emit func(asyncexecmanager.Overview),
) {
	if manager == nil || emit == nil {
		return
	}
	var (
		lastRevision uint64
		emitted      bool
	)
	for {
		changed := manager.ChangeSignal()
		overview := manager.Overview()
		if !emitted || overview.Revision != lastRevision {
			// Keep the lazy manager's empty revision zero internal. Any lifecycle
			// change, including close without executions, is still published.
			if overview.Revision != 0 || len(overview.Executions) != 0 {
				emit(overview)
				lastRevision = overview.Revision
				emitted = true
			}
		}
		if manager.Closed() && overview.RunningCount() == 0 {
			return
		}
		select {
		case <-changed:
			continue
		case <-ctx.Done():
			latest := manager.Overview()
			if !emitted || latest.Revision != lastRevision {
				emit(latest)
			}
			return
		}
	}
}
