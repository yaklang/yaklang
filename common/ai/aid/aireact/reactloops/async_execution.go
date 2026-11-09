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

// GetAsyncExecutionManager delegates to the runtime contract. A loop without an
// invoker retains a task-scoped fallback for isolated loop tests.
func (r *ReActLoop) GetAsyncExecutionManager() (*asyncexecmanager.Manager, error) {
	if r == nil {
		return nil, errors.New("re-act loop is nil")
	}
	if !utils.IsNil(r.invoker) {
		released := r.asyncAdmissionClosed()
		if released && r.invoker.PeekAsyncExecutionManager() == nil {
			return nil, asyncexecmanager.ErrClosed
		}
		return r.invoker.GetAsyncExecutionManager()
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
	sessionOwned := !utils.IsNil(r.invoker)
	manager, err := r.GetAsyncExecutionManager()
	if err != nil {
		return asyncexecmanager.Summary{}, err
	}
	admissionClosed := r.asyncAdmissionClosed()
	if admissionClosed {
		return asyncexecmanager.Summary{}, asyncexecmanager.ErrClosed
	}
	if utils.IsNil(r.invoker) {
		return asyncexecmanager.Summary{}, errors.New("runtime does not support async tool calls")
	}
	task := r.GetCurrentTask()
	name = strings.TrimSpace(name)
	if utils.IsNil(task) || name == "" || params == nil {
		return asyncexecmanager.Summary{}, errors.New("async tool call requires an owner, tool name and explicit parameters")
	}
	executionID := "exec-" + ksuid.New().String()
	opts = append(opts, aicommon.WithToolCaller_AsyncExecutionID(executionID))
	resource, cancel, err := r.invoker.PrepareAsyncToolCall(task, name, params, opts...)
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
	if sessionOwned && r.invoker.AsyncDirectAnswerRequested() {
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
	if sessionOwned {
		releaseAdmission, allowed := r.invoker.BeginAsyncExecutionAdmission()
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
	if sessionOwned {
		r.invoker.TrackAsyncExecutionTask(task, receipt.ID)
	}
	return receipt, err
}

func (r *ReActLoop) asyncAdmissionClosed() bool {
	r.onReleaseMutex.Lock()
	closed := r.asyncAdmissionClosedLocked()
	r.onReleaseMutex.Unlock()
	if !utils.IsNil(r.invoker) && r.invoker.AsyncDirectAnswerRequested() {
		return true
	}
	return closed
}

func (r *ReActLoop) asyncAdmissionClosedLocked() bool {
	return r.released || r.asyncDirectAnswerRequested
}
