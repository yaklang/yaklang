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
	Revision     uint64                     `json:"revision"`
	TaskID       string                     `json:"task_id,omitempty"`
	LoopName     string                     `json:"loop_name,omitempty"`
	RunningCount int                        `json:"running_count"`
	UpdatedAt    int64                      `json:"updated_at"`
	Executions   []asyncexecmanager.Summary `json:"executions"`
	Execution    *asyncexecmanager.Summary  `json:"execution,omitempty"`
	Error        string                     `json:"error,omitempty"`
}

// AsyncExecutionSnapshot returns a metadata-only view of this loop's manager.
// It never creates a manager or samples resource output.
func (r *ReActLoop) AsyncExecutionSnapshot() AsyncExecutionSnapshot {
	if r == nil {
		return AsyncExecutionSnapshot{
			UpdatedAt:  time.Now().UnixMilli(),
			Executions: make([]asyncexecmanager.Summary, 0),
		}
	}
	manager := r.PeekAsyncExecutionManager()
	if manager == nil {
		return r.asyncExecutionSnapshotFromOverview(asyncexecmanager.Overview{})
	}
	return r.asyncExecutionSnapshotFromOverview(manager.Overview())
}

func (r *ReActLoop) asyncExecutionSnapshotFromOverview(overview asyncexecmanager.Overview) AsyncExecutionSnapshot {
	snapshot := AsyncExecutionSnapshot{
		Revision:     overview.Revision,
		RunningCount: overview.RunningCount(),
		UpdatedAt:    time.Now().UnixMilli(),
		Executions:   overview.Executions,
	}
	if r == nil {
		snapshot.Executions = make([]asyncexecmanager.Summary, 0)
		return snapshot
	}
	if task := r.GetCurrentTask(); task != nil {
		snapshot.TaskID = task.GetId()
	}
	snapshot.LoopName = r.GetLoopName()
	if snapshot.Executions == nil {
		snapshot.Executions = make([]asyncexecmanager.Summary, 0)
	}
	return snapshot
}

// PeekAsyncExecutionManager returns the existing manager without changing lazy
// manager creation semantics.
func (r *ReActLoop) PeekAsyncExecutionManager() *asyncexecmanager.Manager {
	if r == nil {
		return nil
	}
	r.onReleaseMutex.Lock()
	defer r.onReleaseMutex.Unlock()
	return r.asyncExecutionManager
}

// StopAsyncExecution requests cancellation of one execution owned by this
// loop. A stopping state is not terminal; callers should consume the next
// pushed snapshots until cleanup finishes.
func (r *ReActLoop) StopAsyncExecution(executeID string) (asyncexecmanager.Summary, error) {
	manager := r.PeekAsyncExecutionManager()
	if manager == nil {
		return asyncexecmanager.Summary{}, asyncexecmanager.ErrNotFound
	}
	return manager.Stop(executeID)
}

func (r *ReActLoop) watchAsyncExecutionManager(ctx context.Context, manager *asyncexecmanager.Manager) {
	if r == nil || manager == nil {
		return
	}
	var (
		lastRevision uint64
		emitted      bool
	)
	for {
		// Capture the next signal before sampling state. Any transition after the
		// sample closes this channel; any transition before it leaves the channel
		// already closed, so neither case can be lost.
		changed := manager.ChangeSignal()
		overview := manager.Overview()
		if !emitted || overview.Revision != lastRevision {
			// The lazy manager starts with an empty revision zero state. Keep it
			// internal, but publish any lifecycle state or a later closed snapshot.
			if overview.Revision != 0 || len(overview.Executions) != 0 {
				r.emitAsyncExecutionSnapshot(overview)
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
			// Close is propagated asynchronously from the owner context. Publish
			// the latest retained state before the task-local emitter is torn down.
			latest := manager.Overview()
			if !emitted || latest.Revision != lastRevision {
				r.emitAsyncExecutionSnapshot(latest)
			}
			return
		}
	}
}

func (r *ReActLoop) emitAsyncExecutionSnapshot(overview asyncexecmanager.Overview) {
	if r == nil || r.GetEmitter() == nil {
		return
	}
	_, _ = r.GetEmitter().EmitStructured(AsyncExecutionSyncNodeID, r.asyncExecutionSnapshotFromOverview(overview))
}
