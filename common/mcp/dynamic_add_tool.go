package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/mcp/yakcliconvert"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	_ "github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
	_ "github.com/yaklang/yaklang/common/yak/static_analyzer/ssa_option"
)

func init() {
	AddGlobalToolSet("dynamic",
		WithTool(mcp.NewTool("dynamic_add_tool",
			mcp.WithDescription("dynamic add tool from yak script content"),
			mcp.WithString("name",
				mcp.Description("The new tool name"),
				mcp.Required(),
			),
			mcp.WithString("description",
				mcp.Description("The new tool description"),
				mcp.Required(),
			),
			mcp.WithString("code",
				mcp.Description("The yak script content"),
				mcp.Required(),
			),
		), handleDynamicAddTool),
	)
}

func handleDynamicAddTool(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments
		toolName := utils.MapGetString(args, "name")
		content := utils.MapGetString(args, "code")
		description := utils.MapGetString(args, "description")

		prog, err := static_analyzer.SSAParse(content, "yak")
		if err != nil {
			return nil, err
		}

		tool := yakcliconvert.ConvertCliParameterToTool(toolName, prog)
		// use script help first
		if tool.Description == "" {
			tool.Description = description
		}
		s.server.AddTool(tool, s.execYakScriptWrapper(toolName, content))

		return NewCommonCallToolResult(fmt.Sprintf("add tool[%s] success", toolName))
	}
}

func (s *MCPServer) execYakScriptWrapper(toolName, content string) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		req := ypb.DebugPluginRequest{
			PluginType: "yak",
			Code:       content,
		}
		args := request.Params.Arguments
		req.ExecParams = make([]*ypb.KVPair, 0, len(args))
		for k, v := range args {
			req.ExecParams = append(req.ExecParams, &ypb.KVPair{
				Key:   k,
				Value: utils.InterfaceToString(v),
			})
		}
		stream, err := s.grpcClient.DebugPlugin(ctx, &req)
		if err != nil {
			return nil, utils.Wrapf(err, "failed to exec yak script[%s]", toolName)
		}

		var progressToken mcp.ProgressToken
		meta := request.Params.Meta
		if meta != nil {
			progressToken = meta.ProgressToken
		}

		results := make([]any, 0, 4)
		for {
			exec, err := stream.Recv()
			if err != nil {
				if !errors.Is(err, io.EOF) {
					results = append(results, mcp.TextContent{
						Type: "text",
						Text: fmt.Sprintf("[Error] %v", err),
					})
				}
				break
			}

			content := string(exec.Message)
			content = handleExecMessage(content)

			results = append(results, mcp.TextContent{
				Type: "text",
				Text: content,
			})
			s.server.SendNotificationToClient(fmt.Sprintf("%s/info", toolName), map[string]any{
				"content":       content,
				"progress":      exec.Progress,
				"progressToken": progressToken,
			})
		}
		if len(results) == 0 {
			results = append(results, mcp.TextContent{
				Type: "text",
				Text: fmt.Sprintf("[System] Exec yak script[%s] completed with no output", toolName),
			})
		}

		return NewCommonCallToolResult(results)
	}
}
