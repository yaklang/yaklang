package mcp

import (
	"context"

	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var mitmFilterDataItemToolOptions = []mcp.ToolOption{
	mcp.WithString("matcherType", mcp.Description("Matcher type: word, regexp, glob, ...")),
	mcp.WithStringArray("group", mcp.Description("Match pattern groups")),
	mcp.WithString("ruleName", mcp.Description("Rule display name")),
}

var mitmFilterDataToolOptions = []mcp.ToolOption{
	mcp.WithStructArray("includeHostnames", []mcp.PropertyOption{mcp.Description("Hostnames to include")},
		mitmFilterDataItemToolOptions...),
	mcp.WithStructArray("excludeHostnames", []mcp.PropertyOption{mcp.Description("Hostnames to exclude")},
		mitmFilterDataItemToolOptions...),
	mcp.WithStructArray("includeSuffix", []mcp.PropertyOption{mcp.Description("URL suffixes to include")},
		mitmFilterDataItemToolOptions...),
	mcp.WithStructArray("excludeSuffix", []mcp.PropertyOption{mcp.Description("URL suffixes to exclude")},
		mitmFilterDataItemToolOptions...),
	mcp.WithStructArray("includeUri", []mcp.PropertyOption{mcp.Description("URIs to include")},
		mitmFilterDataItemToolOptions...),
	mcp.WithStructArray("excludeUri", []mcp.PropertyOption{mcp.Description("URIs to exclude")},
		mitmFilterDataItemToolOptions...),
	mcp.WithStructArray("excludeMethods", []mcp.PropertyOption{mcp.Description("HTTP methods to exclude")},
		mitmFilterDataItemToolOptions...),
	mcp.WithStructArray("excludeMIME", []mcp.PropertyOption{mcp.Description("Content-types to exclude")},
		mitmFilterDataItemToolOptions...),
	mcp.WithBool("filterBundledStaticJS", mcp.Description("Filter bundled/minified static JS (default true)")),
}

var mitmSetFilterToolOptions = []mcp.ToolOption{
	mcp.WithStruct("filterData", []mcp.PropertyOption{
		mcp.Description("MITM traffic filter rules"),
		mcp.Required(),
	}, mitmFilterDataToolOptions...),
	mcp.WithStringArray("includeHostname", mcp.Description("Legacy flat hostname include list")),
	mcp.WithStringArray("excludeHostname", mcp.Description("Legacy flat hostname exclude list")),
	mcp.WithStringArray("includeSuffix", mcp.Description("Legacy flat suffix include list")),
	mcp.WithStringArray("excludeSuffix", mcp.Description("Legacy flat suffix exclude list")),
	mcp.WithStringArray("excludeMethod", mcp.Description("Legacy flat HTTP method exclude list")),
	mcp.WithStringArray("excludeContentTypes", mcp.Description("Legacy flat content-type exclude list")),
	mcp.WithStringArray("excludeUri", mcp.Description("Legacy flat URI exclude list")),
	mcp.WithStringArray("includeUri", mcp.Description("Legacy flat URI include list")),
}

var mitmContentReplacerToolOptions = []mcp.ToolOption{
	mcp.WithString("rule", mcp.Description("Match pattern (regex by default; set exactMatch for literal)")),
	mcp.WithString("result", mcp.Description("Replacement text or highlight template")),
	mcp.WithBool("noReplace", mcp.Description("Highlight only, do not replace")),
	mcp.WithString("color", mcp.Description("Highlight color")),
	mcp.WithBool("enableForRequest", mcp.Description("Apply to request")),
	mcp.WithBool("enableForResponse", mcp.Description("Apply to response")),
	mcp.WithBool("enableForHeader", mcp.Description("Apply to headers")),
	mcp.WithBool("enableForBody", mcp.Description("Apply to body")),
	mcp.WithBool("enableForURI", mcp.Description("Apply to URI")),
	mcp.WithBool("disabled", mcp.Description("Disable this replacer")),
	mcp.WithString("verboseName", mcp.Description("Display name")),
	mcp.WithBool("exactMatch", mcp.Description("Treat rule as literal text, not regex")),
	mcp.WithBool("drop", mcp.Description("Drop matched packets")),
}

func init() {
	AddGlobalToolSet("mitm",
		WithTool(mcp.NewTool("get_mitm_filter",
			mcp.WithDescription("Get MITM traffic filter configuration"),
		), unaryEmptyToolHandler(func(ctx context.Context, s *MCPServer) (any, error) {
			return s.grpcClient.GetMITMFilter(ctx, &ypb.Empty{})
		}, "failed to get mitm filter")),

		WithTool(mcp.NewTool("set_mitm_filter",
			append([]mcp.ToolOption{
				mcp.WithDescription("Set MITM traffic filter configuration"),
			}, mitmSetFilterToolOptions...)...,
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.SetMITMFilterRequest) (any, error) {
			return s.grpcClient.SetMITMFilter(ctx, req)
		}, "failed to set mitm filter")),

		WithTool(mcp.NewTool("query_mitm_replacer_rules",
			mcp.WithDescription("Query MITM content replacer rules"),
			mcp.WithString("keyword", mcp.Description("Fuzzy search on replacer rules")),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.QueryMITMReplacerRulesRequest) (any, error) {
			return s.grpcClient.QueryMITMReplacerRules(ctx, req)
		}, "failed to query mitm replacer rules")),

		WithTool(mcp.NewTool("get_current_rules",
			mcp.WithDescription("Get currently active MITM content replacer rules"),
		), unaryEmptyToolHandler(func(ctx context.Context, s *MCPServer) (any, error) {
			return s.grpcClient.GetCurrentRules(ctx, &ypb.Empty{})
		}, "failed to get current mitm rules")),

		WithTool(mcp.NewTool("set_current_rules",
			mcp.WithDescription("Set active MITM content replacer rules"),
			mcp.WithStructArray("rules", []mcp.PropertyOption{
				mcp.Description("MITM content replacers"),
			}, mitmContentReplacerToolOptions...),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.MITMContentReplacers) (any, error) {
			_, err := s.grpcClient.SetCurrentRules(ctx, req)
			if err != nil {
				return nil, err
			}
			return "set current mitm rules success", nil
		}, "failed to set current mitm rules")),

		WithTool(mcp.NewTool("download_mitm_cert",
			mcp.WithDescription("Download MITM CA certificate PEM"),
		), unaryEmptyToolHandler(func(ctx context.Context, s *MCPServer) (any, error) {
			return s.grpcClient.DownloadMITMCert(ctx, &ypb.Empty{})
		}, "failed to download mitm cert")),

		WithTool(mcp.NewTool("start_mitm_v2",
			mcp.WithDescription("Start MITM v2 proxy (runs in background)"),
			mcp.WithString("host", mcp.Description("Listen host"), mcp.Default("127.0.0.1")),
			mcp.WithNumber("port", mcp.Description("Listen port"), mcp.Required()),
			mcp.WithString("downstreamProxy", mcp.Description("Upstream proxy URL, e.g. http://127.0.0.1:7890")),
			mcp.WithBool("enableHttp2", mcp.Description("Enable HTTP/2")),
			mcp.WithBool("filterWebsocket", mcp.Description("Filter websocket traffic")),
			mcp.WithNumber("maxContentLength", mcp.Description("Max captured content length in bytes")),
			mcp.WithBool("disableSystemProxy", mcp.Description("Ignore system proxy environment variables")),
		), handleStartMITMV2),
	)
}

func handleStartMITMV2(s *MCPServer) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var req ypb.MITMV2Request
		if err := decodeYakRequest(request.Params.Arguments, &req); err != nil {
			return nil, err
		}
		summary := map[string]any{
			"host": req.GetHost(),
			"port": req.GetPort(),
		}
		bgCtx := context.Background()
		stream, err := s.grpcClient.MITMV2(bgCtx)
		if err != nil {
			return nil, utils.Wrap(err, "failed to start mitm v2")
		}
		if err := stream.Send(&req); err != nil {
			return nil, utils.Wrap(err, "failed to send mitm v2 request")
		}
		storeBackgroundStreamStatus("start_mitm_v2", summary)
		go func() {
			for {
				rsp, err := stream.Recv()
				if err != nil {
					return
				}
				if rsp != nil && rsp.HaveMessage && rsp.Message != nil {
					content := handleExecMessage(string(rsp.Message.Message))
					if content != "" {
						appendBackgroundStreamLog("start_mitm_v2", content)
					}
				}
			}
		}()
		return NewCommonCallToolResult(map[string]any{
			"status":  "started",
			"name":    "start_mitm_v2",
			"summary": summary,
		})
	}
}
