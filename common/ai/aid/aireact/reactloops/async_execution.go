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

// GetAsyncExecutionManager returns the manager owned by the loop's ReAct runtime.
// The loop never owns or creates a manager itself.
func (r *ReActLoop) GetAsyncExecutionManager() (*asyncexecmanager.Manager, error) {
	if r == nil {
		return nil, errors.New("re-act loop is nil")
	}
	if utils.IsNil(r.invoker) {
		return nil, errors.New("async execution requires an AI invoke runtime")
	}
	released := r.asyncAdmissionClosed()
	if released && !r.invoker.HasAsyncExecutionManager() {
		return nil, asyncexecmanager.ErrClosed
	}
	return r.invoker.GetAsyncExecutionManager()
}

// StartAsyncToolCall prepares the caller through the runtime and submits it to
// the session manager. It returns an execution receipt, never a tool result.
// The source task is fixed for review and result attribution.
func (r *ReActLoop) StartAsyncToolCall(name string, params aitool.InvokeParams, opts ...aicommon.ToolCallerOption) (asyncexecmanager.Summary, error) {
	manager, err := r.GetAsyncExecutionManager()
	if err != nil {
		return asyncexecmanager.Summary{}, err
	}
	invoker := r.invoker
	if utils.IsNil(invoker) {
		return asyncexecmanager.Summary{}, errors.New("async execution requires an AI invoke runtime")
	}
	admissionClosed := r.asyncAdmissionClosed()
	if admissionClosed {
		return asyncexecmanager.Summary{}, asyncexecmanager.ErrClosed
	}
	task := r.GetCurrentTask()
	name = strings.TrimSpace(name)
	if utils.IsNil(task) || name == "" || params == nil {
		return asyncexecmanager.Summary{}, errors.New("async tool call requires an owner, tool name and explicit parameters")
	}
	executionID := "exec-" + ksuid.New().String()
	opts = append(opts, aicommon.WithToolCaller_AsyncExecutionID(executionID))
	resource, cancel, err := invoker.PrepareAsyncToolCall(task, name, params, opts...)
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
	admissionClosed = r.released
	r.onReleaseMutex.Unlock()
	if invoker.AsyncDirectAnswerRequested() {
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
	releaseAdmission, allowed := invoker.BeginAsyncExecutionAdmission()
	if !allowed {
		cancel()
		return asyncexecmanager.Summary{}, asyncexecmanager.ErrClosed
	}
	defer releaseAdmission()
	receipt, err := manager.Start(asyncexecmanager.Request{
		ID: executionID, Resource: resource,
		Metadata: asyncexecmanager.Metadata{Name: name, Attributes: attributes},
	})
	if err != nil {
		cancel()
		return receipt, err
	}
	invoker.TrackAsyncExecutionTask(task, receipt.ID)
	return receipt, err
}

type reviewedAsyncToolPreparer interface {
	PrepareReviewedAsyncToolCall(task aicommon.AIStatefulTask, review *aicommon.ToolCallReviewResult) (asyncexecmanager.Resource, func(), error)
}

// StartReviewedAsyncToolCall submits an already reviewed proposal. Review must
// complete before this method creates a Manager execution.
func (r *ReActLoop) StartReviewedAsyncToolCall(review *aicommon.ToolCallReviewResult) (asyncexecmanager.Summary, error) {
	if r == nil || utils.IsNil(r.invoker) {
		return asyncexecmanager.Summary{}, errors.New("async execution requires an AI invoke runtime")
	}
	task := r.GetCurrentTask()
	if utils.IsNil(task) || review == nil || review.Tool == nil || review.Params == nil {
		return asyncexecmanager.Summary{}, errors.New("async tool call requires an owner and approved review")
	}
	preparer, ok := r.invoker.(reviewedAsyncToolPreparer)
	if !ok {
		return asyncexecmanager.Summary{}, errors.New("runtime does not expose approved async tool preparation")
	}
	manager, err := r.GetAsyncExecutionManager()
	if err != nil {
		return asyncexecmanager.Summary{}, err
	}
	if r.asyncAdmissionClosed() || r.invoker.AsyncDirectAnswerRequested() {
		return asyncexecmanager.Summary{}, asyncexecmanager.ErrClosed
	}

	executionID := "exec-" + ksuid.New().String()
	review.BindAsyncExecutionID(executionID)
	resource, cancel, err := preparer.PrepareReviewedAsyncToolCall(task, review)
	if err != nil {
		if cancel != nil {
			cancel()
		}
		return asyncexecmanager.Summary{}, err
	}
	if cancel == nil {
		review.Abort(errors.New("async tool preparation returned no cancellation handle"))
		return asyncexecmanager.Summary{}, errors.New("async tool preparation must provide cancellation")
	}
	if ctx := task.GetContext(); ctx != nil {
		if err := ctx.Err(); err != nil {
			cancel()
			review.Abort(err)
			return asyncexecmanager.Summary{}, err
		}
	}
	if r.asyncAdmissionClosed() {
		cancel()
		review.Abort(asyncexecmanager.ErrClosed)
		return asyncexecmanager.Summary{}, asyncexecmanager.ErrClosed
	}
	releaseAdmission, allowed := r.invoker.BeginAsyncExecutionAdmission()
	if !allowed {
		cancel()
		review.Abort(asyncexecmanager.ErrClosed)
		return asyncexecmanager.Summary{}, asyncexecmanager.ErrClosed
	}
	defer releaseAdmission()

	attributes := map[string]string{"task_id": task.GetId(), "loop": r.GetLoopName()}
	if identity, ok := resource.(interface {
		CallToolID() string
		ResultID() int64
	}); ok {
		attributes["call_tool_id"] = identity.CallToolID()
		attributes["result_id"] = fmt.Sprint(identity.ResultID())
	}
	receipt, err := manager.Start(asyncexecmanager.Request{
		ID: executionID, Resource: resource,
		Metadata: asyncexecmanager.Metadata{Name: review.Tool.Name, Attributes: attributes},
	})
	if err != nil {
		review.Abort(err)
		return receipt, err
	}
	r.invoker.TrackAsyncExecutionTask(task, receipt.ID)
	return receipt, nil
}

func (r *ReActLoop) asyncAdmissionClosed() bool {
	r.onReleaseMutex.Lock()
	closed := r.released
	r.onReleaseMutex.Unlock()
	if !utils.IsNil(r.invoker) && r.invoker.AsyncDirectAnswerRequested() {
		return true
	}
	return closed
}
