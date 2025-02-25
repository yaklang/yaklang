package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/go-viper/mapstructure/v2"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	AddGlobalToolSet("hybrid_scan",
		WithTool(mcp.NewTool(string("hybrid_scan"),
			mcp.WithDescription("Initiate a hybrid scan, which entails executing Yak scripts across multiple targets"),
			mcp.WithStruct("targets",
				[]mcp.PropertyOption{
					mcp.Description("Hybrid scan input targets"),
					mcp.Required(),
				},
				mcp.WithStringArray("input",
					mcp.Description("target input"),
				),
				mcp.WithStringArray("inputFile",
					mcp.Description("List of input files for the scan"),
				),
			),
			mcp.WithStruct("plugin",
				[]mcp.PropertyOption{
					mcp.Description("Hybrid scan plugin configuration"),
					mcp.Required(),
				},
				mcp.WithStringArray("pluginNames",
					mcp.Description("List of plugin names to use"),
				),
				mcp.WithStruct("filter", []mcp.PropertyOption{
					mcp.Description("Query YakScript request for filtering"),
				}, filterYakScriptToolOptions...),
			),
			mcp.WithNumber("concurrent",
				mcp.Description("Number of concurrent scans"),
				mcp.Default(20),
				mcp.Required(),
			),
			mcp.WithNumber("totalTimeoutSecond",
				mcp.Description("Total timeout in seconds for the scan"),
				mcp.Min(0),
				mcp.Default(0),
			),
			mcp.WithString("proxy",
				mcp.Description("Proxy for the scan. e.g. http://127.0.0.1:1080"),
			),
			mcp.WithNumber("singleTimeoutSecond",
				mcp.Description("Single request timeout in seconds"),
				mcp.Min(0),
				mcp.Default(0),
			),
		), handleHybridScan),
	)
}

func handleHybridScan(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		var req ypb.HybridScanRequest
		decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
			DecodeHook: arrayToStringHook,
			Result:     &req,
		})
		if err != nil {
			return nil, utils.Wrap(err, "BUG: new map structure decoder error")
		}
		err = decoder.Decode(request.Params.Arguments)
		if err != nil {
			return nil, utils.Wrap(err, "invalid argument")
		}
		var progressToken mcp.ProgressToken
		meta := request.Params.Meta
		if meta != nil {
			progressToken = meta.ProgressToken
		}

		stream, err := s.grpcClient.HybridScan(ctx)
		if err != nil {
			return nil, utils.Wrap(err, "failed to start hybrid scan")
		}
		source := uuid.NewString()
		err = stream.Send(&ypb.HybridScanRequest{
			Control:              true,
			Detach:               true,
			HybridScanMode:       "new",
			HybridScanTaskSource: source,
		})
		if err != nil {
			return nil, utils.Wrap(err, "failed to send request")
		}
		err = stream.Send(&req)
		if err != nil {
			return nil, utils.Wrap(err, "failed to send request")
		}

		results := make([]any, 0, 4)
		progress := 0.0
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

			// progress
			if rsp.TotalTasks > 0 {
				newProgress := float64(rsp.FinishedTasks) / float64(rsp.TotalTasks)
				if newProgress > progress {
					s.server.SendNotificationToClient("hybrid_scan/progress", map[string]any{
						"progress":      float64(rsp.FinishedTasks) / float64(rsp.TotalTasks),
						"progressToken": progressToken,
					})
					progress = newProgress
				}
			}

			// info
			exec := rsp.ExecResult
			if exec == nil {
				continue
			}
			content := string(exec.Message)
			// handle complex message
			msgContent := gjson.GetBytes(exec.Message, "content")
			level := msgContent.Get("level").String()
			switch level {
			case "feature-status-card-data":
				continue
			case "info", "json", "json-risk":
				// use content directly
				content = msgContent.Get("data").String()
			}
			if content == "" {
				continue
			}
			// risk 特殊处理
			if level == "json-risk" {

				newContent, err := sjson.Set(content, "Request", strconv.Quote(msgContent.Get("Request").String()))
				if err == nil {
					content = newContent
				}
				newContent, err = sjson.Set(content, "Response", strconv.Quote(msgContent.Get("Response").String()))
				if err == nil {
					content = newContent
				}
			}
			results = append(results, mcp.TextContent{
				Type: "text",
				Text: content,
			})
			s.server.SendNotificationToClient("hybrid_scan/info", map[string]any{
				"content":       content,
				"progressToken": progressToken,
			})
		}
		if len(results) == 0 {
			results = append(results, mcp.TextContent{
				Type: "text",
				Text: "[System] Hybrid scan completed with no output",
			})
		}

		return NewCommonCallToolResult(results)
	}
}
