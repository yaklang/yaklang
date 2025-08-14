package aid

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

var ToolCallWatcher = []*ToolUseReviewSuggestion{
	{
		Value:             "enough-cancel",
		Suggestion:        "跳过",
		SuggestionEnglish: "Tool output is sufficient, can cancel tool execution and continue with the next task",
	},
}

var (
	ToolCallAction_Enough_Cancel = "enough-cancel"
	ToolCallAction_Finish        = "finish"
)

func (t *AiTask) InvokeTool(targetTool *aitool.Tool, callToolParams aitool.InvokeParams, callToolId string, handleResultUserCancel, handleResultErr func(any), stdoutWriter, stderrWriter io.Writer) (*aitool.ToolResult, error) {
	c := t.config
	seq := c.AcquireId()
	if ret, ok := yakit.GetToolCallCheckpoint(c.GetDB(), c.id, seq); ok { // todo rerun
		if ret.Finished {
			return aiddb.AiCheckPointGetToolResult(ret), nil
		}
	}
	toolCheckpoint := c.createToolCallCheckpoint(seq)
	err := c.submitToolCallRequestCheckpoint(toolCheckpoint, targetTool, callToolParams)
	if err != nil {
		return nil, err
	}

	ep := c.epm.createEndpointWithEventType(schema.EVENT_TYPE_TOOL_CALL_WATCHER)
	c.EmitToolCallWatcher(callToolId, ep.id, targetTool, callToolParams)
	ctx, cancel := context.WithCancel(c.ctx)
	defer cancel()

	newToolCallRes := func() *aitool.ToolResult {
		return &aitool.ToolResult{
			Param:       callToolParams,
			Name:        targetTool.Name,
			Description: targetTool.Description,
			ToolCallID:  callToolId,
		}
	}

	toolCallSuccess := func(result *aitool.ToolExecutionResult) (*aitool.ToolResult, error) {
		res := newToolCallRes()
		res.Success = true
		res.Data = result
		err = c.submitToolCallResponse(toolCheckpoint, res)
		if err != nil {
			return nil, err
		}
		return res, nil
	}

	toolCallErr := func(err error) (*aitool.ToolResult, error) {
		handleResultErr(err)
		res := newToolCallRes()
		res.Error = fmt.Sprintf("工具执行失败: %v", err)
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
		case ToolCallAction_Enough_Cancel:
			cancel()
			handleResultUserCancel("用户取消工具调用，继续后续任务")
		case ToolCallAction_Finish:
		default:
			handleResultErr(fmt.Sprintf("用户未选择有效的操作，无法继续工具调用: %v", userSuggestion))
		}
	}()

	execResult, execErr := targetTool.InvokeWithParams(callToolParams,
		aitool.WithStdout(stdoutWriter),
		aitool.WithStderr(stderrWriter),
		aitool.WithContext(ctx),
		aitool.WithErrorCallback(toolCallErr),
		aitool.WithResultCallback(toolCallSuccess),
		aitool.WithCancelCallback(toolCallCancel),
		aitool.WithRuntimeConfig(&aitool.ToolRuntimeConfig{
			RuntimeID: c.id,
			FeedBacker: func(result *ypb.ExecResult) error {
				c.EmitYakitExecResult(result)
				return nil
			},
		}),
	)
	ep.ActiveWithParams(ctx, map[string]any{"suggestion": "finish"})
	c.ReleaseInteractiveEvent(ep.id, map[string]any{"suggestion": "finish"})

	return execResult, execErr
}
