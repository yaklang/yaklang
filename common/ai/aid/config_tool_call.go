package aid

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aiddb"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"golang.org/x/net/context"
	"io"
)

var ToolCallWatcher = []*ToolUseReviewSuggestion{
	{
		Value:             "enough-cancel",
		Suggestion:        "工具输出足够，可以取消工具执行，继续后面的任务",
		SuggestionEnglish: "Tool output is sufficient, can cancel tool execution and continue with the next task",
	},
}

var (
	ToolCallAction_Enough_Cancel = "enough-cancel"
	ToolCallAction_Finish        = "finish"
)

func (c *Config) toolCallOpts(toolCallID string, cancelHandle, resultErrHandle func(any), stdoutBuf, stderrBuf io.Writer) []aitool.ToolInvokeOptions {
	return []aitool.ToolInvokeOptions{
		aitool.WithStdout(stdoutBuf),
		aitool.WithStderr(stderrBuf),
		aitool.WithInvokeHook(func(t *aitool.Tool, params map[string]any, config *aitool.ToolInvokeConfig) (*aitool.ToolResult, error) {
			seq := c.AcquireId()
			if ret, ok := yakit.GetToolCallCheckpoint(c.GetDB(), c.id, seq); ok { // todo rerun
				if ret.Finished {
					return aiddb.AiCheckPointGetToolResult(ret), nil
				}
			}
			toolCheckpoint := c.createToolCallCheckpoint(seq)
			err := c.submitToolCallRequestCheckpoint(toolCheckpoint, t, params)
			if err != nil {
				return nil, err
			}
			ctx, cancel := context.WithCancel(c.ctx)
			defer cancel()
			ep := c.epm.createEndpointWithEventType(EVENT_TYPE_TOOL_CALL_WATCHER)
			c.EmitToolCallWatcher(toolCallID, ep.id, t, params)

			toolCallSuccess := func(result *aitool.ToolExecutionResult) (*aitool.ToolResult, error) {
				res := &aitool.ToolResult{
					Param:       params,
					Name:        t.Name,
					Description: t.Description,
					Success:     true,
					Data:        result,
				}
				err = c.submitToolCallResponse(toolCheckpoint, res)
				if err != nil {
					return nil, err
				}
				return res, nil
			}

			toolCallErr := func(err error) (*aitool.ToolResult, error) {
				resultErrHandle(err)
				return &aitool.ToolResult{
					Param:       params,
					Name:        t.Name,
					Description: t.Description,
					Success:     false,
					Error:       fmt.Sprintf("工具执行失败: %v", err),
				}, err
			}

			outBuf, errBuf := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
			stdOutWriter := io.MultiWriter(config.GetStdout(), outBuf)
			stdErrWriter := io.MultiWriter(config.GetStderr(), errBuf)

			var execResult *aitool.ToolExecutionResult
			var execErr error
			go func() {
				execResult, execErr = t.ExecuteToolWithCapture(ctx, params, stdOutWriter, stdErrWriter)
				ep.ActiveWithParams(ctx, map[string]any{"suggestion": "finish"})
				c.ReleaseInteractiveEvent(ep.id, map[string]any{"suggestion": "finish"})
			}()

			ep.WaitContext(ctx)
			userSuggestion := ep.GetParams()
			switch userSuggestion.GetString("suggestion") {
			case ToolCallAction_Enough_Cancel:
				cancel()
				cancelHandle("用户取消工具调用，继续后续任务")
				return toolCallSuccess(&aitool.ToolExecutionResult{
					Stdout: outBuf.String(),
					Stderr: errBuf.String(),
				})
			case ToolCallAction_Finish:
				if execErr != nil {
					return toolCallErr(execErr)
				}
				if execResult == nil {
					return toolCallSuccess(&aitool.ToolExecutionResult{
						Stdout: outBuf.String(),
						Stderr: errBuf.String(),
					})
				}
				return toolCallSuccess(execResult)
			default:
				actionErr := utils.Errorf("tool call unknown user suggestion: %s", userSuggestion.GetString("suggestion"))
				return toolCallErr(actionErr)
			}
		}),
	}
}
