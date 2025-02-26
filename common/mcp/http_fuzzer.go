package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/go-viper/mapstructure/v2"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	AddGlobalToolSet("http_fuzzer",
		WithTool(mcp.NewTool("http_fuzzer",
			mcp.WithDescription("Send HTTP packet(s) based on the given parameters, allow use fuzztag directly"),
			mcp.WithString("request",
				mcp.Description("The raw HTTP request packet to be fuzzed, allow fuzztag"),
				mcp.Required(),
			),
			mcp.WithNumber("concurrent",
				mcp.Description("Number of concurrent requests to send"),
				mcp.Required(),
				mcp.Default(20),
				mcp.Min(1),
			),
			mcp.WithBool("isHttps",
				mcp.Description("Indicates if the request should use HTTPS"),
				mcp.Required(),
			),
			mcp.WithBool("isGmTls",
				mcp.Description("Indicates if the request should use GM TLS (Chinese cryptographic standard)"),
			),
			mcp.WithString("fuzzTagMode",
				mcp.Description("The fuzztag mode"),
				mcp.Enum("close", "standard"),
				mcp.Required(),
			),
			mcp.WithString("proxy",
				mcp.Description("Proxy for the request. e.g. http://127.0.0.1:1080"),
			),
			mcp.WithNumber("perRequestTimeoutSeconds",
				mcp.Description("Timeout in seconds for each request"),
				mcp.Min(0),
			),
			mcp.WithBool("noSystemProxy",
				mcp.Description("Disables the use of system proxy"),
			),
			mcp.WithString("actualAddr",
				mcp.Description("Actual Address to send, if not set, use Host Header as target"),
			),
			mcp.WithBool("noFollowRedirect",
				mcp.Description("Disables following redirects"),
			),
			mcp.WithNumber("redirectTimes",
				mcp.Description("Maximum number of redirects to follow"),
				mcp.Min(0),
			),
			mcp.WithBool("noFixContentLength",
				mcp.Description("Disables automatic fixing response, such as Content-Length header"),
			),
			mcp.WithString("responseCharset",
				mcp.Description("Charset to use for the response. e.g. gb18030"),
			),
			mcp.WithStringArray("dnsServers",
				mcp.Description("Custom DNS servers to use for the request. e.g. 8.8.8.8"),
			),
			mcp.WithKVPairs("etcHosts",
				mcp.Description("Custom /etc/hosts entries to use for the request"),
			),
			mcp.WithNumber("repeatTimes",
				mcp.Description("Number of times to repeat the request"),
				mcp.Min(1),
			),
			mcp.WithStringArray("batchTarget",
				mcp.Description("Batch target to be used for the request"),
			),
			mcp.WithBool("batchTargetFile",
				mcp.Description("Indicates if the batch target is provided as a file"),
			),
			mcp.WithBool("disableUseConnPool",
				mcp.Description("Disables the use of connection pool"),
			),
		), handleHTTPFuzzer),
	)
}

func handleHTTPFuzzer(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		var req ypb.FuzzerRequest
		decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
			DecodeHook: decodeHook,
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
		stream, err := s.grpcClient.HTTPFuzzer(ctx, &req)
		if err != nil {
			return nil, utils.Wrap(err, "failed to start http fuzzer")
		}
		req.DisableHotPatch = true

		results := make([]any, 0, 4)
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
			m := map[string]any{
				"host":                rsp.Host,
				"request":             string(rsp.RequestRaw),
				"response":            string(rsp.ResponseRaw),
				"durationMs":          rsp.TotalDurationMs,
				"firstByteDurationMs": rsp.FirstByteDurationMs,
			}
			if rsp.Discard {
				m["discard"] = true
			}
			if !rsp.Ok {
				m["ok"] = false
				m["err"] = rsp.Reason
			}
			if len(rsp.Payloads) > 0 {
				m["payloads"] = rsp.Payloads
			}
			if rsp.IsTooLargeResponse {
				m["isTooLargeResponse"] = true
				m["large_response_header_file"] = rsp.TooLargeResponseHeaderFile
				m["large_response_body_file"] = rsp.TooLargeResponseBodyFile
			}
			contentBytes, err := json.Marshal(m)
			content := string(contentBytes)
			if err == nil {
				results = append(results, mcp.TextContent{
					Type: "text",
					Text: content,
				})
				s.server.SendNotificationToClient("http_fuzzer/info", map[string]any{
					"content":       content,
					"progressToken": progressToken,
				})
			}

		}
		if len(results) == 0 {
			results = append(results, mcp.TextContent{
				Type: "text",
				Text: "[System] HTTP Fuzzer completed with no output",
			})
		}

		return NewCommonCallToolResult(results)
	}
}
