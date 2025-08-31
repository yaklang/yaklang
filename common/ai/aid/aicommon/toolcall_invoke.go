package aicommon

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/yaklang/yaklang/common/ai/aid/aiddb"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	ToolCallAction_Enough_Cancel = "enough-cancel"
	ToolCallAction_Finish        = "finish"
)

func (a *ToolCaller) invoke(
	tool *aitool.Tool,
	params aitool.InvokeParams,
	userCancel func(reason any),
	reportError func(err any),
	stdoutWriter, stderrWriter io.Writer,
) (*aitool.ToolResult, error) {
	c := a.config
	e := a.emitter

	seq := c.AcquireId()
	if ret, ok := yakit.GetToolCallCheckpoint(c.GetDB(), c.GetRuntimeId(), seq); ok {
		if ret.Finished {
			return aiddb.AiCheckPointGetToolResult(ret), nil
		}
	}
	toolCheckpoint := c.CreateToolCallCheckpoint(seq)
	err := c.SubmitCheckpointRequest(toolCheckpoint, map[string]any{
		"tool_name": tool.Name,
		"param":     params,
	})
	if err != nil {
		return nil, err
	}

	epm := c.GetEndpointManager()
	ep := epm.CreateEndpointWithEventType(schema.EVENT_TYPE_TOOL_CALL_WATCHER)
	e.EmitToolCallWatcher(a.callToolId, ep.GetId(), tool, params)
	ctx, cancel := context.WithCancel(c.GetContext())
	defer cancel()

	newToolCallRes := func() *aitool.ToolResult {
		return &aitool.ToolResult{
			Param:       params,
			Name:        tool.Name,
			Description: tool.Description,
			ToolCallID:  a.callToolId,
		}
	}

	toolCallSuccess := func(result *aitool.ToolExecutionResult) (*aitool.ToolResult, error) {
		res := newToolCallRes()
		res.Success = true
		res.Data = result
		err = c.SubmitCheckpointResponse(toolCheckpoint, res)
		if err != nil {
			return nil, err
		}
		return res, nil
	}

	toolCallErr := func(err error) (*aitool.ToolResult, error) {
		reportError(err)
		res := newToolCallRes()
		res.Error = fmt.Sprintf("tool execution failed: %v", err)
		return res, err
	}

	toolCallCancel := func(result *aitool.ToolExecutionResult, err error) (*aitool.ToolExecutionResult, error) {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return result, nil
		}
		return result, err
	}

	go func() {
		ep.WaitContext(ctx)
		userSuggestion := ep.GetParams()
		switch userSuggestion.GetString("suggestion") {
		case string(ToolCallAction_Enough_Cancel):
			cancel()
			userCancel("user cancelled the tool call, continuing with the next task")
		case ToolCallAction_Finish:
		default:
			reportError(fmt.Sprintf("user did not select a valid action, cannot continue tool call: %v", userSuggestion))
		}
	}()

	noRuntimeId := !params.Has("runtime_id")
	if noRuntimeId {
		params.Set("runtime_id", a.runtimeId)
	}

	execResult, execErr := tool.InvokeWithParams(
		params,
		aitool.WithStdout(stdoutWriter),
		aitool.WithStderr(stderrWriter),
		aitool.WithContext(ctx),
		aitool.WithErrorCallback(toolCallErr),
		aitool.WithResultCallback(toolCallSuccess),
		aitool.WithCancelCallback(toolCallCancel),
		aitool.WithRuntimeConfig(&aitool.ToolRuntimeConfig{
			RuntimeID: c.GetRuntimeId(),
			FeedBacker: func(result *ypb.ExecResult) error {
				e.EmitYakitExecResult(result)
				return nil
			},
		}),
	)
	ep.ActiveWithParams(ctx, map[string]any{"suggestion": "finish"})
	reqs := map[string]any{"suggestion": "finish"}
	e.EmitInteractiveRelease(ep.GetId(), reqs)
	c.CallAfterInteractiveEventReleased(ep.GetId(), reqs)

	if execResult != nil && noRuntimeId {
		if r, ok := execResult.Param.(aitool.InvokeParams); ok {
			if r.Has("runtime_id") {
				delete(r, "runtime_id")
			}
		}
	}

	return execResult, execErr
}
