package aicommon

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/asyncexecmanager"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

// toolCallResource schedules the original caller without retaining output or
// result copies. Inspector returns that caller's existing observation interface.
type toolCallResource struct {
	caller   *ToolCaller
	tool     *aitool.Tool
	params   aitool.InvokeParams
	onResult func(*aitool.ToolResult, bool, error)
	started  atomic.Bool
}

var _ asyncexecmanager.Resource = (*toolCallResource)(nil)

// NewAsyncResource transfers an unused caller to one async execution. Configure
// task, emitter, review and scheduling hooks before calling this method; after
// success, do not invoke or reconfigure the caller directly. The tool definition
// must remain immutable; JSON parameters are copied at this preparation boundary.
// Construction neither starts a tool nor changes the legacy synchronous path.
// An optional result handler runs once after the original call settles, before
// Execute returns. It is for owner task/timeline bookkeeping, not observation.
func (t *ToolCaller) NewAsyncResource(tool *aitool.Tool, params aitool.InvokeParams, onResult ...func(*aitool.ToolResult, bool, error)) (asyncexecmanager.Resource, error) {
	if t == nil || t.ctx == nil || t.cancel == nil || t.m == nil {
		return nil, fmt.Errorf("%w: an initialized tool caller is required", asyncexecmanager.ErrInvalidResource)
	}
	if tool == nil || tool.Tool == nil || params == nil {
		return nil, fmt.Errorf("%w: a tool and explicit parameters are required", asyncexecmanager.ErrInvalidResource)
	}
	if utils.IsNil(t.task) {
		return nil, fmt.Errorf("%w: a tool caller task is required", asyncexecmanager.ErrInvalidResource)
	}
	if len(onResult) > 1 {
		return nil, fmt.Errorf("%w: at most one tool result handler is allowed", asyncexecmanager.ErrInvalidResource)
	}
	params = cloneEndpointParams(params)
	t.m.Lock()
	defer t.m.Unlock()
	if t.asyncResourceBound || !t.startTime.IsZero() {
		return nil, fmt.Errorf("%w: tool caller is already in use", asyncexecmanager.ErrInvalidResource)
	}
	t.asyncResourceBound = true
	// The resource cannot settle while plugin workers still own its writers.
	t.waitForCallbackOnCancel = true
	resource := &toolCallResource{caller: t, tool: tool, params: params}
	if len(onResult) == 1 {
		resource.onResult = onResult[0]
	}
	return resource, nil
}

// Cancel releases this caller's context, including an unsubmitted or pending
// async caller. Running async resources still join callback cleanup on return.
func (t *ToolCaller) Cancel() {
	if t != nil && t.cancel != nil {
		t.cancel()
	}
}

func (r *toolCallResource) Inspector() asyncexecmanager.Inspector {
	return r.caller.Inspector()
}

func (r *toolCallResource) Execute(ctx context.Context) error {
	if ctx == nil {
		return errors.New("tool execution context is required")
	}
	if !r.started.CompareAndSwap(false, true) {
		return errors.New("tool call resource has already executed")
	}
	stop := context.AfterFunc(ctx, r.caller.cancel)
	defer stop()
	defer r.caller.cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := r.caller.ctx.Err(); err != nil {
		return err
	}
	result, directlyAnswer, err := r.caller.CallToolWithExistedParams(r.tool, r.params)
	if r.onResult != nil {
		r.onResult(result, directlyAnswer, err)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if err != nil {
		return err
	}
	if err := r.caller.ctx.Err(); err != nil {
		return err
	}
	if directlyAnswer || result == nil {
		return errors.New("tool call did not produce a result")
	}
	if !result.Success {
		if result.Error != "" {
			return errors.New(result.Error)
		}
		return errors.New("tool call returned an unsuccessful result")
	}
	return nil
}
