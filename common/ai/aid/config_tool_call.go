package aid

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aiddb"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func (c *Config) toolCallOpts(stdoutBuf, stderrBuf *bytes.Buffer) []aitool.ToolInvokeOptions {
	return []aitool.ToolInvokeOptions{
		aitool.WithStdout(stdoutBuf),
		aitool.WithStderr(stderrBuf),
		aitool.WithChatToAiFunc(aitool.ChatToAiFuncType(c.toolAICallback)),
		aitool.WithInvokeHook(func(t *aitool.Tool, params map[string]any, config *aitool.ToolInvokeConfig) (*aitool.ToolResult, error) {
			seq := c.AcquireId()
			if ret, ok := aiddb.GetToolCallCheckpoint(c.GetDB(), c.id, seq); ok { // todo rerun
				if ret.Finished {
					return aiddb.AiCheckPointGetToolResult(ret), nil
				}
			}
			toolCheckpoint := c.createToolCallCheckpoint(seq)
			err := c.submitToolCallRequestCheckpoint(toolCheckpoint, t, params)
			if err != nil {
				return nil, err
			}

			var execResult *aitool.ToolExecutionResult
			execResult, err = t.ExecuteToolWithCapture(params, config.GetStdout(), config.GetStdout())
			if err != nil {
				return &aitool.ToolResult{
					Param:       params,
					Name:        t.Name,
					Description: t.Description,
					Success:     false,
					Error:       fmt.Sprintf("工具执行失败: %v", err),
				}, err
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
