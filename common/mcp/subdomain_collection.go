package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	AddGlobalToolSet("subdomain",
		WithTool(mcp.NewTool("subdomain_collection",
			mcp.WithDescription("Collects subdomains for a given target domain"),
			mcp.WithString("target",
				mcp.Description("The target domain to collect subdomains for"),
				mcp.Required(),
			),
			mcp.WithBool("notRecursive",
				mcp.Description("Specifies whether to perform recursive subdomain enumeration"),
			),
			mcp.WithBool("wildcardToStop",
				mcp.Description("Specifies whether to stop when encountering wildcard DNS records"),
			),
		), handleSubdomainCollection))
}

func handleSubdomainCollection(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		return s.downloadAndExecYakScript(ctx,
			"subdomain_collection",
			"子域名收集",
			"8cc4491d-5b77-43ea-b6ea-3f78b99b73e2",
			request,
			func(stream ypb.Yak_DebugPluginClient, _ string) (*mcp.CallToolResult, error) {
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
					if !exec.IsMessage {
						continue
					}

					content := string(exec.Message)
					content = handleExecMessage(content)

					// 放行 error 级消息（如子域名扫描因 DNS 被劫持而中止的报错）：
					// 这类消息没有 data.Domain 字段，但需要让 AI/用户看到中止原因，
					// 否则只会看到空结果而无任何提示。
					if gjson.Get(content, "content.level").String() == "error" {
						errData := gjson.Get(content, "content.data").String()
						if errData == "" {
							errData = content
						}
						results = append(results, mcp.TextContent{
							Type: "text",
							Text: fmt.Sprintf("[Error] %s", errData),
						})
						s.notificationServer(ctx).SendNotificationToClient("notifications/message", map[string]any{
							"level": "error",
							"data":  errData,
						})
						continue
					}

					if !gjson.Get(content, "data.Domain").Exists() {
						continue
					}
					content = gjson.Get(content, "data").String()
					newContent, err := sjson.Delete(content, "uuid")
					if err == nil {
						content = newContent
					}

					results = append(results, mcp.TextContent{
						Type: "text",
						Text: content,
					})
					s.notificationServer(ctx).SendNotificationToClient("notifications/message", map[string]any{
						"level": "info",
						"data":  content,
					})
				}
				if len(results) == 0 {
					results = append(results, mcp.TextContent{
						Type: "text",
						Text: "[System] Script execution completed with no output",
					})
				}
				return NewCommonCallToolResult(results)
			},
		)
	}
}
