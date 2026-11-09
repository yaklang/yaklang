package reactloops

import (
	"errors"
	"fmt"
	"strings"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/asyncexecmanager"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

// GetAsyncExecutionManager delegates to a session-scoped invoker when
// available. Standalone loops retain the task-scoped fallback used by tests and
// non-ReAct runtimes.
func (r *ReActLoop) GetAsyncExecutionManager() (*asyncexecmanager.Manager, error) {
	if r == nil {
		return nil, errors.New("re-act loop is nil")
	}
	if provider, ok := r.invoker.(aicommon.AsyncExecutionManagerProvider); ok {
		released := r.asyncAdmissionClosed()
		if released && provider.PeekAsyncExecutionManager() == nil {
			return nil, asyncexecmanager.ErrClosed
		}
		return provider.GetAsyncExecutionManager()
	}
	task := r.GetCurrentTask()
	r.onReleaseMutex.Lock()
	if r.asyncExecutionManager != nil {
		manager := r.asyncExecutionManager
		r.onReleaseMutex.Unlock()
		return manager, nil
	}
	if r.released || r.asyncDirectAnswerRequested {
		r.onReleaseMutex.Unlock()
		return nil, asyncexecmanager.ErrClosed
	}
	if utils.IsNil(task) {
		r.onReleaseMutex.Unlock()
		return nil, errors.New("async execution requires an owning task")
	}
	manager, err := asyncexecmanager.New(task.GetContext(), asyncexecmanager.Config{})
	if err != nil {
		r.onReleaseMutex.Unlock()
		return nil, err
	}
	r.asyncExecutionManager = manager
	taskContext := task.GetContext()
	r.onReleaseMutex.Unlock()
	go WatchAsyncExecutionManager(taskContext, manager, func(overview asyncexecmanager.Overview) {
		if r.GetEmitter() != nil {
			_, _ = r.GetEmitter().EmitStructured(
				AsyncExecutionSyncNodeID,
				BuildAsyncExecutionSnapshot(overview, r.persistentSessionID(), r.currentTaskID()),
			)
		}
	})
	return manager, nil
}

// StartAsyncToolCall prepares the caller through the runtime and submits it to
// the loop or session manager. It returns an execution receipt, never a tool
// result. The source task is fixed for review and result attribution.
func (r *ReActLoop) StartAsyncToolCall(name string, params aitool.InvokeParams, opts ...aicommon.ToolCallerOption) (asyncexecmanager.Summary, error) {
	_, sessionOwned := r.invoker.(aicommon.AsyncExecutionManagerProvider)
	manager, err := r.GetAsyncExecutionManager()
	if err != nil {
		return asyncexecmanager.Summary{}, err
	}
	admissionClosed := r.asyncAdmissionClosed()
	if admissionClosed {
		return asyncexecmanager.Summary{}, asyncexecmanager.ErrClosed
	}
	runtime, ok := r.invoker.(aicommon.AsyncToolCallRuntime)
	if !ok || utils.IsNil(runtime) {
		return asyncexecmanager.Summary{}, errors.New("runtime does not support async tool calls")
	}
	task := r.GetCurrentTask()
	name = strings.TrimSpace(name)
	if utils.IsNil(task) || name == "" || params == nil {
		return asyncexecmanager.Summary{}, errors.New("async tool call requires an owner, tool name and explicit parameters")
	}
	executionID := "exec-" + ksuid.New().String()
	opts = append(opts, aicommon.WithToolCaller_AsyncExecutionID(executionID))
	resource, cancel, err := runtime.PrepareAsyncToolCall(task, name, params, opts...)
	if err != nil {
		if cancel != nil {
			cancel()
		}
		return asyncexecmanager.Summary{}, err
	}
	if cancel == nil {
		return asyncexecmanager.Summary{}, errors.New("async tool preparation must provide cancellation")
	}
	r.onReleaseMutex.Lock()
	admissionClosed = r.asyncAdmissionClosedLocked()
	if !sessionOwned {
		r.onRelease = append(r.onRelease, cancel)
	}
	r.onReleaseMutex.Unlock()
	if provider, ok := r.invoker.(aicommon.AsyncDirectAnswerProvider); ok &&
		provider.AsyncDirectAnswerRequested() {
		admissionClosed = true
	}
	if admissionClosed {
		cancel()
		return asyncexecmanager.Summary{}, asyncexecmanager.ErrClosed
	}
	attributes := map[string]string{"task_id": task.GetId(), "loop": r.GetLoopName()}
	if identity, ok := resource.(interface {
		CallToolID() string
		ResultID() int64
	}); ok {
		attributes["call_tool_id"] = identity.CallToolID()
		attributes["result_id"] = fmt.Sprint(identity.ResultID())
	}
	if provider, ok := r.invoker.(aicommon.AsyncDirectAnswerProvider); ok {
		releaseAdmission, allowed := provider.BeginAsyncExecutionAdmission()
		if !allowed {
			cancel()
			return asyncexecmanager.Summary{}, asyncexecmanager.ErrClosed
		}
		defer releaseAdmission()
	}
	receipt, err := manager.Start(asyncexecmanager.Request{
		ID: executionID, Resource: resource,
		Metadata: asyncexecmanager.Metadata{Name: name, Attributes: attributes},
	})
	if err != nil {
		cancel()
		return receipt, err
	}
	if tracker, ok := r.invoker.(aicommon.AsyncExecutionTaskTracker); ok {
		tracker.TrackAsyncExecutionTask(task, receipt.ID)
	}
	return receipt, err
}

func (r *ReActLoop) asyncAdmissionClosed() bool {
	r.onReleaseMutex.Lock()
	closed := r.asyncAdmissionClosedLocked()
	r.onReleaseMutex.Unlock()
	if provider, ok := r.invoker.(aicommon.AsyncDirectAnswerProvider); ok &&
		provider.AsyncDirectAnswerRequested() {
		return true
	}
	return closed
}

func (r *ReActLoop) asyncAdmissionClosedLocked() bool {
	return r.released || r.asyncDirectAnswerRequested
}
