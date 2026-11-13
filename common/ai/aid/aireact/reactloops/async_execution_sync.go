package reactloops

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/asyncexecmanager"
	"github.com/yaklang/yaklang/common/utils"
)

const AsyncExecutionSyncNodeID = "async_execution"

type AsyncExecutionSnapshot = aicommon.AsyncExecutionSnapshot

// BuildAsyncExecutionSnapshot builds the shared metadata-only wire payload.
func BuildAsyncExecutionSnapshot(overview asyncexecmanager.Overview, sessionID, currentTaskID string) AsyncExecutionSnapshot {
	return aicommon.BuildAsyncExecutionSnapshot(overview, sessionID, currentTaskID)
}

// AsyncExecutionSnapshot returns the session manager snapshot from this loop's
// runtime. The loop itself never owns manager state.
func (r *ReActLoop) AsyncExecutionSnapshot() AsyncExecutionSnapshot {
	if r == nil {
		return BuildAsyncExecutionSnapshot(asyncexecmanager.Overview{}, "", "")
	}
	if utils.IsNil(r.invoker) {
		return BuildAsyncExecutionSnapshot(asyncexecmanager.Overview{}, r.persistentSessionID(), r.currentTaskID())
	}
	return r.invoker.AsyncExecutionSnapshot()
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

// HasAsyncExecutionManager reports whether the loop's runtime already owns a
// manager. It never creates one.
func (r *ReActLoop) HasAsyncExecutionManager() bool {
	if r == nil || utils.IsNil(r.invoker) {
		return false
	}
	return r.invoker.HasAsyncExecutionManager()
}

// StopAsyncExecution requests cancellation of one execution owned by the loop's
// runtime. A stopping state is not terminal.
func (r *ReActLoop) StopAsyncExecution(executeID string) (asyncexecmanager.Summary, error) {
	if r != nil && !utils.IsNil(r.invoker) {
		return r.invoker.StopAsyncExecution(executeID)
	}
	return asyncexecmanager.Summary{}, asyncexecmanager.ErrNotFound
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
