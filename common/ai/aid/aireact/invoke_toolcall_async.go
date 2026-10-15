package aireact

import (
	"errors"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/asyncexecmanager"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

var _ aicommon.AsyncToolCallRuntime = (*ReAct)(nil)

// PrepareAsyncToolCall reuses the existing lookup, owner guards, parameter
// mutators and caller setup. Review and invocation remain inside Execute.
func (r *ReAct) PrepareAsyncToolCall(task aicommon.AIStatefulTask, name string, params aitool.InvokeParams, opts ...aicommon.ToolCallerOption) (asyncexecmanager.Resource, func(), error) {
	if r == nil || r.config == nil || utils.IsNil(task) || params == nil {
		return nil, nil, errors.New("async tool call requires an initialized runtime, owner and explicit parameters")
	}
	ctx := task.GetContext()
	if ctx == nil {
		return nil, nil, errors.New("async tool call owner context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	tool, err := r.resolveToolForCall(ctx, name)
	if err != nil {
		return nil, nil, err
	}
	if ownerLoop, _ := task.GetReActLoop().(*reactloops.ReActLoop); ownerLoop != nil {
		if allow, message := reactloops.CheckToolInvokeGuard(ownerLoop, name, params); !allow {
			return nil, nil, utils.Error(message)
		}
		params = reactloops.ApplyToolInvokeParamsMutators(ownerLoop, name, params)
	}
	// Assign before execution so the Inspector and task/timeline use the same ID.
	opts = append(opts,
		aicommon.WithToolCaller_StatsSource(aicommon.StatsSourceToolDirect),
		aicommon.WithToolCaller_Task(task), aicommon.WithToolCaller_InvokeRuntime(r))
	caller, err := r.newToolCallerForCall(ctx, task, name, opts...)
	if err != nil {
		return nil, nil, err
	}
	resource, err := caller.NewAsyncResource(tool, params, func(result *aitool.ToolResult, directly bool, callErr error) {
		r.finishDetachedToolCall(task, result, directly, callErr)
	})
	if err != nil {
		caller.Cancel()
		return nil, nil, err
	}
	return resource, caller.Cancel, nil
}
