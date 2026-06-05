package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	mcpmcp "github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/utils"
)

// convertAIToolToMCPHandler wraps an aitool.Tool's Callback into a server.ToolHandlerFunc
// suitable for registration in the MCP server. The aitool's stdout/stderr are captured
// and merged into the MCP TextContent response.
func convertAIToolToMCPHandler(aiTool *aitool.Tool) ToolHandlerWrapperFunc {
	return func(s *MCPServer) server.ToolHandlerFunc {
		return func(ctx context.Context, request mcpmcp.CallToolRequest) (*mcpmcp.CallToolResult, error) {
			if aiTool.Callback == nil {
				return nil, utils.Errorf("aitool %q has no callback registered", aiTool.Name)
			}

			params := aitool.InvokeParams(request.Params.Arguments)

			var stdoutBuf, stderrBuf bytes.Buffer

			rawResult, execErr := aiTool.Callback(ctx, params, nil, &stdoutBuf, &stderrBuf)

			contents := make([]any, 0, 4)

			// stdout output
			if stdoutBuf.Len() > 0 {
				contents = append(contents, mcpmcp.TextContent{
					Type: "text",
					Text: stdoutBuf.String(),
				})
			}

			// structured return value
			if rawResult != nil {
				var resultText string
				switch v := rawResult.(type) {
				case string:
					resultText = v
				case []byte:
					resultText = string(v)
				case io.Reader:
					b, err := io.ReadAll(v)
					if err != nil {
						log.Warnf("aitool %q: failed to read result reader: %v", aiTool.Name, err)
					} else {
						resultText = string(b)
					}
				default:
					b, err := json.Marshal(rawResult)
					if err != nil {
						resultText = fmt.Sprintf("%v", rawResult)
					} else {
						resultText = string(b)
					}
				}
				if resultText != "" {
					contents = append(contents, mcpmcp.TextContent{
						Type: "text",
						Text: resultText,
					})
				}
			}

			// execution error: report inside result per MCP spec so the LLM can self-correct
			if execErr != nil {
				errText := execErr.Error()
				if stderrBuf.Len() > 0 {
					errText = stderrBuf.String() + "\n" + errText
				}
				return &mcpmcp.CallToolResult{
					Content: []any{
						mcpmcp.TextContent{
							Type: "text",
							Text: fmt.Sprintf("[Error] %s", errText),
						},
					},
					IsError: true,
				}, nil
			}

			if stderrBuf.Len() > 0 {
				contents = append(contents, mcpmcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("[Stderr] %s", stderrBuf.String()),
				})
			}

			if len(contents) == 0 {
				contents = append(contents, mcpmcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("[System] Tool %q completed with no output", aiTool.Name),
				})
			}

			return &mcpmcp.CallToolResult{Content: contents}, nil
		}
	}
}

// WithAITools registers a list of aitool.Tool instances into the MCP server.
// Each aitool is converted to a standard MCP tool using its embedded mcp.Tool
// definition (name, description, inputSchema) and its Callback as the handler.
// Tools that lack a Callback are skipped with a warning.
func WithAITools(tools ...*aitool.Tool) McpServerOption {
	return func(cfg *MCPServerConfig) error {
		for _, t := range tools {
			if t == nil {
				continue
			}
			if t.Callback == nil {
				log.Warnf("aitool %q has no callback, skipping MCP registration", t.Name)
				continue
			}
			cfg.extraAITools[t.Name] = &ToolWithHandler{
				tool:    t.Tool, // *mcp.Tool embedded in aitool.Tool
				handler: convertAIToolToMCPHandler(t),
			}
			if t.BridgeMCPClient != nil {
				cfg.trackBridgeClientCloser(t.BridgeMCPClient)
			}
		}
		return nil
	}
}
