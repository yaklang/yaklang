package reactloops

import (
	"errors"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/asyncexecmanager"
	"github.com/yaklang/yaklang/common/utils"
)

// GetAsyncExecutionManager binds one manager to the current task on first use.
// Later calls reuse it even if currentTask changes. Release closes admission and
// requests cancellation; the retained manager remains available for inspection.
// TBD: actions still execute tools synchronously; this is the integration seam.
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
