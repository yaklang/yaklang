package aireact

import (
	"context"
	"errors"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/asyncexecmanager"
	"github.com/yaklang/yaklang/common/log"
)

const asyncExecutionShutdownTimeout = 5 * time.Second

// GetAsyncExecutionManager lazily creates the one manager owned by this ReAct
// session. Queued tasks and their ReActLoop instances come and go, while
// background executions remain manageable across user turns.
func (r *ReAct) GetAsyncExecutionManager() (*asyncexecmanager.Manager, error) {
	if r == nil || r.config == nil {
		return nil, errors.New("re-act runtime is not initialized")
	}
	r.asyncExecutionMu.Lock()
	if r.asyncExecutionManager != nil {
		manager := r.asyncExecutionManager
		r.asyncExecutionMu.Unlock()
		return manager, nil
	}
	if r.AsyncDirectAnswerRequested() {
		r.asyncExecutionMu.Unlock()
		return nil, asyncexecmanager.ErrClosed
	}
	ctx := r.config.GetContext()
	if ctx == nil {
		r.asyncExecutionMu.Unlock()
		return nil, errors.New("async execution session context is required")
	}
	if err := ctx.Err(); err != nil {
		r.asyncExecutionMu.Unlock()
		return nil, err
	}
	manager, err := asyncexecmanager.New(ctx, asyncexecmanager.Config{})
	if err != nil {
		r.asyncExecutionMu.Unlock()
		return nil, err
	}
	r.asyncExecutionManager = manager
	r.asyncExecutionMu.Unlock()

	go reactloops.WatchAsyncExecutionManager(ctx, manager, func(overview asyncexecmanager.Overview) {
		if r.Emitter != nil {
			_, _ = r.EmitStructured(
				reactloops.AsyncExecutionSyncNodeID,
				reactloops.BuildAsyncExecutionSnapshot(overview, r.config.PersistentSessionId, r.GetCurrentTaskId()),
			)
		}
	})
	r.trackAsyncExecutionShutdown(ctx, manager)
	return manager, nil
}

// existingAsyncExecutionManager returns the existing session manager without
// creating one.
func (r *ReAct) existingAsyncExecutionManager() *asyncexecmanager.Manager {
	if r == nil {
		return nil
	}
	r.asyncExecutionMu.Lock()
	defer r.asyncExecutionMu.Unlock()
	return r.asyncExecutionManager
}

func (r *ReAct) HasAsyncExecutionManager() bool {
	return r.existingAsyncExecutionManager() != nil
}

// AsyncExecutionSnapshot returns the session-scoped metadata view consumed by
// push events and sync responses.
func (r *ReAct) AsyncExecutionSnapshot() aicommon.AsyncExecutionSnapshot {
	if r == nil {
		return reactloops.BuildAsyncExecutionSnapshot(asyncexecmanager.Overview{}, "", "")
	}
	manager := r.existingAsyncExecutionManager()
	if manager == nil {
		sessionID := ""
		if r.config != nil {
			sessionID = r.config.PersistentSessionId
		}
		return reactloops.BuildAsyncExecutionSnapshot(asyncexecmanager.Overview{}, sessionID, r.GetCurrentTaskId())
	}
	return reactloops.BuildAsyncExecutionSnapshot(manager.Overview(), r.config.PersistentSessionId, r.GetCurrentTaskId())
}

func (r *ReAct) StopAsyncExecution(executeID string) (asyncexecmanager.Summary, error) {
	manager := r.existingAsyncExecutionManager()
	if manager == nil {
		return asyncexecmanager.Summary{}, asyncexecmanager.ErrNotFound
	}
	return manager.Stop(executeID)
}

// TrackAsyncExecutionTask applies the cross-turn cancellation rule once the
// execution has been admitted. Natural completion leaves background work alive;
// explicit cancellation or an aborted/skipped task stops its executions.
func (r *ReAct) TrackAsyncExecutionTask(task aicommon.AIStatefulTask, executeID string) {
	if r == nil || task == nil || executeID == "" {
		return
	}
	ctx := task.GetContext()
	if ctx == nil || ctx.Done() == nil {
		return
	}
	context.AfterFunc(ctx, func() {
		if task.GetStatus() == aicommon.AITaskState_Completed {
			return
		}
		if _, err := r.StopAsyncExecution(executeID); err != nil &&
			!errors.Is(err, asyncexecmanager.ErrNotFound) {
			log.Warnf("stop async execution %s after task %s cancellation: %v", executeID, task.GetId(), err)
		}
	})
}

// AsyncDirectAnswerSignal is session-scoped so a review started by an earlier
// task can wake the loop currently serving the next user turn.
func (r *ReAct) AsyncDirectAnswerSignal() <-chan struct{} {
	r.asyncDirectAnswerMu.Lock()
	defer r.asyncDirectAnswerMu.Unlock()
	if r.asyncDirectAnswer == nil {
		r.asyncDirectAnswer = make(chan struct{})
	}
	return r.asyncDirectAnswer
}

func (r *ReAct) RequestAsyncDirectAnswer() {
	r.asyncDirectAnswerMu.Lock()
	if r.asyncDirectRequested {
		r.asyncDirectAnswerMu.Unlock()
		return
	}
	if r.asyncDirectAnswer == nil {
		r.asyncDirectAnswer = make(chan struct{})
	}
	r.asyncDirectRequested = true
	close(r.asyncDirectAnswer)
	r.asyncDirectAnswerMu.Unlock()
	if manager := r.existingAsyncExecutionManager(); manager != nil {
		manager.StopAll()
	}
}

func (r *ReAct) AsyncDirectAnswerRequested() bool {
	r.asyncDirectAnswerMu.Lock()
	defer r.asyncDirectAnswerMu.Unlock()
	return r.asyncDirectRequested
}

func (r *ReAct) BeginAsyncExecutionAdmission() (func(), bool) {
	r.asyncDirectAnswerMu.Lock()
	if r.asyncDirectRequested {
		r.asyncDirectAnswerMu.Unlock()
		return func() {}, false
	}
	return r.asyncDirectAnswerMu.Unlock, true
}

// ConsumeAsyncDirectAnswer acknowledges one request and resets the session
// signal so a later background review can request another answer.
func (r *ReAct) ConsumeAsyncDirectAnswer() bool {
	r.asyncDirectAnswerMu.Lock()
	defer r.asyncDirectAnswerMu.Unlock()
	if !r.asyncDirectRequested {
		return false
	}
	r.asyncDirectRequested = false
	r.asyncDirectAnswer = make(chan struct{})
	return true
}

// StopAsyncExecutionsForTask cancels background executions explicitly
// attributed to a user-cancelled task. Natural task completion does not call
// this method, so intended cross-turn work remains alive.
func (r *ReAct) StopAsyncExecutionsForTask(taskID string) []asyncexecmanager.Summary {
	manager := r.existingAsyncExecutionManager()
	if manager == nil || taskID == "" {
		return nil
	}
	var stopped []asyncexecmanager.Summary
	for _, execution := range manager.List() {
		if execution.State.IsTerminal() || execution.Metadata.Attributes["task_id"] != taskID {
			continue
		}
		if summary, err := manager.Stop(execution.ID); err == nil {
			stopped = append(stopped, summary)
		}
	}
	return stopped
}

// trackAsyncExecutionShutdown makes session lifecycle waiting include manager
// cleanup without tying the manager to an individual task context.
func (r *ReAct) trackAsyncExecutionShutdown(ctx context.Context, manager *asyncexecmanager.Manager) {
	done := ctx.Done()
	if done == nil {
		return
	}
	if r.lifecycleWG == nil {
		go func() {
			<-done
			r.shutdownAsyncExecutionManager(manager)
		}()
		return
	}
	r.lifecycleWG.Add(1)
	go func() {
		defer r.lifecycleWG.Done()
		<-done
		r.shutdownAsyncExecutionManager(manager)
	}()
}

func (r *ReAct) shutdownAsyncExecutionManager(manager *asyncexecmanager.Manager) {
	if manager == nil {
		return
	}
	manager.Close()
	ctx, cancel := context.WithTimeout(context.Background(), asyncExecutionShutdownTimeout)
	defer cancel()
	_ = manager.Shutdown(ctx)
}
