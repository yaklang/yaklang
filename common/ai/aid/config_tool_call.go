package aid

import (
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aiddb"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"golang.org/x/net/context"
	"io"
)

var ToolCallWatcher = []*ToolUseReviewSuggestion{
	{
		Value:             "stop-tool-call",
		Suggestion:        "手动停止工具执行",
		SuggestionEnglish: "Manually stop tool execution",
	},
}

func (c *Config) toolCallOpts(stdoutBuf, stderrBuf io.Writer) []aitool.ToolInvokeOptions {
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
			c.EmitToolCallWatcher(ep.id, t, params)

			go func() {
				ep.WaitContext(ctx)
				select {
				case <-ctx.Done():
					c.ReleaseInteractiveEvent(ep.id, nil)
				default:
					cancel()
				}
			}()

			var execResult *aitool.ToolExecutionResult
			var execErr error
			var execFished = make(chan struct{})
			go func() {
				execResult, execErr = t.ExecuteToolWithCapture(ctx, params, config.GetStdout(), config.GetStderr())
				close(execFished)
			}()

			select {
			case <-ctx.Done():
			case <-execFished:
			}
			if execErr != nil {
				return &aitool.ToolResult{
					Param:       params,
					Name:        t.Name,
					Description: t.Description,
					Success:     false,
					Error:       fmt.Sprintf("工具执行失败: %v", err),
				}, execErr
			}
			res := &aitool.ToolResult{
				Name:        t.Name,
				Description: t.Description,
				Param:       params,
				Success:     true,
				Data:        execResult,
			}

			err = c.submitToolCallResponse(toolCheckpoint, res)
			if err != nil {
				return nil, err
			}
			return res, nil
		}),
	}
}
