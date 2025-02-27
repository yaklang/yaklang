package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"

	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	AddGlobalToolSet("crawler",
		WithTool(mcp.NewTool("web_crawler",
			mcp.WithDescription("A web crawler for crawling websites"),
			mcp.WithString("target",
				mcp.Description("The target(s) of crawler, supports flexible formats, including IP, domain, hostname, or URL, split by comma"),
				mcp.Required(),
			),
			mcp.WithNumber("maxDepth",
				mcp.Description("Set the maximum depth of the crawler"),
				mcp.Default(4),
			),
			mcp.WithNumber("concurrent",
				mcp.Description("Set the number of concurrent requests"),
				mcp.Default(50),
			),
			mcp.WithNumber("maxLinks",
				mcp.Description("Set the maximum number of URLs the crawler can fetch"),
				mcp.Default(10000),
			),
			mcp.WithNumber("maxRequests",
				mcp.Description("Set the maximum number of requests the crawler can make"),
				mcp.Default(2000),
			),
			mcp.WithNumber("timeoutPerRequest",
				mcp.Description("Set the maximum timeout for each request"),
				mcp.Default(10),
			),
			mcp.WithString("proxy",
				mcp.Description("Set the proxy for the crawler"),
			),
			mcp.WithString("loginUser",
				mcp.Description("Set the username to try for login"),
				mcp.Default("admin"),
			),
			mcp.WithString("loginPass",
				mcp.Description("Set the password to try for login"),
				mcp.Default("password"),
			),
			mcp.WithString("userAgent",
				mcp.Description("Set the user agent for the crawler"),
			),
			mcp.WithNumber("retry",
				mcp.Description("Set the number of retries for failed requests"),
				mcp.Default(2),
			),
			mcp.WithNumber("redirectTimes",
				mcp.Description("Set the maximum number of redirects allowed"),
				mcp.Default(3),
			),
			mcp.WithBool("basicAuth",
				mcp.Description("Enable or disable basic authentication"),
			),
			mcp.WithString("basicAuthUser",
				mcp.Description("Set the username for basic authentication"),
			),
			mcp.WithString("basicAuthPass",
				mcp.Description("Set the password for basic authentication"),
			),
			mcp.WithString("cookie",
				mcp.Description("Set the cookie for the crawler"),
			),
		), handleWebCrawler))
}

func handleWebCrawler(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		return s.commonExecYakScript(ctx,
			"web_crawler",
			"基础爬虫",
			"eb77ddbc-e703-4e95-b59f-41b3b172ce3d",
			request,
			func(stream ypb.Yak_DebugPluginClient, taskName string) (*mcp.CallToolResult, error) {
				results := make([]any, 0, 4)
				runtimeID := ""

				for {
					rsp, err := stream.Recv()
					if err != nil {
						if !errors.Is(err, io.EOF) {
							results = append(results, mcp.TextContent{
								Type: "text",
								Text: fmt.Sprintf("[Error] %v", err),
							})
						}
						break
					}
					// get runtimeID
					if runtimeID == "" && rsp.RuntimeID != "" {
						runtimeID = rsp.RuntimeID
					}
				}
				rsp, err := s.grpcClient.QueryHTTPFlows(ctx, &ypb.QueryHTTPFlowRequest{
					Pagination: &ypb.Paging{
						Page:  1,
						Limit: -1,
					},
					SourceType: "scan",
					RuntimeId:  runtimeID,
				})
				if err != nil {
					return nil, utils.Wrap(err, "failed to query HTTPFlows")
				}
				results = slices.Grow(results, len(rsp.Data))
				for _, flow := range rsp.Data {
					if flow == nil {
						continue
					}
					results = append(results, ypbHTTPFlowToFriendlyHTTPFlow(flow))
				}
				return NewCommonCallToolResult(results)
			},
		)
	}
}
