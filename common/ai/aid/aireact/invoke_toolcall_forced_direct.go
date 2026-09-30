package aireact

import (
	"context"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loopinfra"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func (r *ReAct) executeForcedDirectlyCall(
	ctx context.Context,
	currentTask aicommon.AIStatefulTask,
	tool *aitool.Tool,
	toolName string,
	opt ...aicommon.ToolCallerOption,
) (*aitool.ToolResult, bool, error) {
	params, reason, err := r.requestToolCallParamsForTask(ctx, currentTask, tool, "")
	if err != nil {
		return nil, false, err
	}
	options := append([]aicommon.ToolCallerOption{
		aicommon.WithToolCaller_Reason(reason),
		aicommon.WithToolCaller_OmitResultParamsInTimeline(),
	}, opt...)
	caller, err := r.newToolCallerForCall(ctx, currentTask, toolName, options...)
	if err != nil {
		return nil, false, err
	}
	result, directlyAnswer, err := caller.CallToolWithExistedParams(tool, params)
	if err != nil {
		return nil, false, err
	}
	return r.finalizeToolCallResult(currentTask, result, directlyAnswer)
}

func (r *ReAct) requestToolCallParamsForTask(ctx context.Context, currentTask aicommon.AIStatefulTask, tool *aitool.Tool, feedback string, requestOptions ...aicommon.AIRequestOption) (aitool.InvokeParams, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	if utils.IsNil(tool) {
		return nil, "", utils.Error("parameter request requires a tool")
	}
	toolName := tool.Name
	if allow, message := reactloops.CheckToolInvokeGuard(promptLoopForTask(currentTask), toolName, nil); !allow {
		return nil, "", utils.Error(message)
	}
	mutation := r.config.RecordRecentlyUsedTool(tool)
	if mutation.Upsert == nil && mutation.Reuse == nil {
		return nil, "", utils.Errorf("cannot load schema for tool %q within the tool-cache budget", toolName)
	}
	r.AddToTimeline("tool_schema_load", fmt.Sprintf("Tool %q schema loaded into CACHE_TOOL_CALL; no execution has occurred.", toolName))
	loop := promptLoopForTask(currentTask)
	if loop == nil {
		var err error
		loop, err = reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_DEFAULT, r,
			reactloops.WithFunctionCallActionVariants())
		if err != nil {
			return nil, "", err
		}
		defer loop.Release()
	}
	call, err := loop.RequestForcedToolCall(ctx, currentTask, toolName, feedback, requestOptions...)
	if err != nil {
		return nil, "", err
	}
	defer loop.ClearActionExecutionValues(call.Action)
	params, err := loopinfra.PrepareDirectToolCallParams(loop, call.Action, tool)
	if err != nil {
		return nil, "", err
	}
	if allow, message := reactloops.CheckToolInvokeGuard(promptLoopForTask(currentTask), toolName, params); !allow {
		return nil, "", utils.Error(message)
	}
	reason := call.Action.GetString("directly_call_reason")
	if strings.TrimSpace(reason) == "" {
		reason = call.Action.GetString("human_readable_thought")
	}
	return params, reason, nil
}
