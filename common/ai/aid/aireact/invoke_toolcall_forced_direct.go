package aireact

import (
	"context"
	"fmt"

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
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	mutation := r.config.RecordRecentlyUsedTool(tool)
	if mutation.Upsert == nil && mutation.Reuse == nil {
		return nil, false, utils.Errorf("cannot load schema for tool %q within the tool-cache budget", toolName)
	}
	r.AddToTimeline("tool_schema_load", fmt.Sprintf("Tool %q schema loaded into CACHE_TOOL_CALL; no execution has occurred.", toolName))
	loop := promptLoopForTask(currentTask)
	if loop == nil {
		var err error
		loop, err = reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_DEFAULT, r,
			reactloops.WithFunctionCallActionVariants())
		if err != nil {
			return nil, false, err
		}
		defer loop.Release()
	}
	call, err := loop.RequestForcedToolCall(ctx, currentTask, toolName)
	if err != nil {
		return nil, false, err
	}
	defer loop.ClearActionExecutionValues(call.Action)
	prepare := func(action *aicommon.Action, name string) (aitool.InvokeParams, bool, *aitool.Tool, error) {
		params, err := loopinfra.PrepareDirectToolCallParams(loop, action, tool)
		if err == nil {
			if allow, message := reactloops.CheckToolInvokeGuard(promptLoopForTask(currentTask), name, params); !allow {
				err = utils.Error(message)
			}
		}
		return params, false, tool, err
	}
	return r.directlyCallToolForTask(ctx, currentTask, toolName, call.Action, prepare, opt...)
}
