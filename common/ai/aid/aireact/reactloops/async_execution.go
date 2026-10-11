package reactloops

import (
	"errors"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/asyncexecmanager"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

// GetAsyncExecutionManager binds one manager to the current task on first use.
// Later calls reuse it even if currentTask changes. Release closes admission and
// requests cancellation; the retained manager remains available for inspection.
func (r *ReActLoop) GetAsyncExecutionManager() (*asyncexecmanager.Manager, error) {
	if r == nil {
		return nil, errors.New("re-act loop is nil")
	}
	task := r.GetCurrentTask()
	r.onReleaseMutex.Lock()
	defer r.onReleaseMutex.Unlock()
	if r.asyncExecutionManager != nil {
		return r.asyncExecutionManager, nil
	}
	if r.released {
		return nil, asyncexecmanager.ErrClosed
	}
	if utils.IsNil(task) {
		return nil, errors.New("async execution requires an owning task")
	}
	manager, err := asyncexecmanager.New(task.GetContext(), asyncexecmanager.Config{})
	if err != nil {
		return nil, err
	}
	r.asyncExecutionManager = manager
	return manager, nil
}

// StartAsyncToolCall prepares the caller through the runtime and submits it to
// this loop's manager. It returns an execution receipt, never a tool result.
// The explicit owner task is fixed before preparation or background execution.
func (r *ReActLoop) StartAsyncToolCall(name string, params aitool.InvokeParams, opts ...aicommon.ToolCallerOption) (asyncexecmanager.Summary, error) {
	manager, err := r.GetAsyncExecutionManager()
	if err != nil {
		return asyncexecmanager.Summary{}, err
	}
	r.onReleaseMutex.Lock()
	released := r.released
	r.onReleaseMutex.Unlock()
	if released {
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
	if r.released {
		r.onReleaseMutex.Unlock()
		cancel()
		return asyncexecmanager.Summary{}, asyncexecmanager.ErrClosed
	}
	r.onRelease = append(r.onRelease, cancel)
	r.onReleaseMutex.Unlock()
	receipt, err := manager.Start(asyncexecmanager.Request{
		Resource: resource,
		Metadata: asyncexecmanager.Metadata{Name: name, Attributes: map[string]string{"task_id": task.GetId(), "loop": r.GetLoopName()}},
	})
	if err != nil {
		cancel()
	}
	return receipt, err
}
