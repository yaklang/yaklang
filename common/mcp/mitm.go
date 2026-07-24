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
	mcp.WithBool("filterBundledStaticJS", mcp.Description("Exclude bundled/minified .js static assets from interception (default true)")),
}

var mitmSetFilterToolOptions = []mcp.ToolOption{
	mcp.WithStruct("filterData", []mcp.PropertyOption{
		mcp.Description("MITM traffic filter rules"),
		mcp.Required(),
	}, mitmFilterDataToolOptions...),
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
			mcp.WithDescription("Read MITM capture filter (include/exclude hostnames, URI, methods, MIME); controls which traffic is shown/processed"),
		), unaryEmptyToolHandler(func(ctx context.Context, s *MCPServer) (any, error) {
			return s.grpcClient.GetMITMFilter(ctx, &ypb.Empty{})
		}, "failed to get mitm filter")),

		WithTool(mcp.NewTool("set_mitm_filter",
			append([]mcp.ToolOption{
				mcp.WithDescription("Update MITM capture filter; prefer filterData struct, legacy flat *Hostname/*Suffix fields still supported"),
			}, mitmSetFilterToolOptions...)...,
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.SetMITMFilterRequest) (any, error) {
			return s.grpcClient.SetMITMFilter(ctx, req)
		}, "failed to set mitm filter")),

		WithTool(mcp.NewTool("query_mitm_replacer_rules",
			mcp.WithDescription("Search saved MITM replacer rule library (not necessarily active); keyword optional"),
			mcp.WithString("keyword", mcp.Description("Fuzzy match on rule name or pattern")),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.QueryMITMReplacerRulesRequest) (any, error) {
			return s.grpcClient.QueryMITMReplacerRules(ctx, req)
		}, "failed to query mitm replacer rules")),

		WithTool(mcp.NewTool("get_current_rules",
			mcp.WithDescription("Get MITM replacer rules currently applied to live interception"),
		), unaryEmptyToolHandler(func(ctx context.Context, s *MCPServer) (any, error) {
			return s.grpcClient.GetCurrentRules(ctx, &ypb.Empty{})
		}, "failed to get current mitm rules")),

		WithTool(mcp.NewTool("set_current_rules",
			mcp.WithDescription("Replace active MITM replacers immediately on running proxy; use get_current_rules first to preserve existing"),
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
			mcp.WithDescription("Download MITM root CA PEM; install in client trust store before pointing browser proxy to start_mitm_v2"),
		), unaryEmptyToolHandler(func(ctx context.Context, s *MCPServer) (any, error) {
			return s.grpcClient.DownloadMITMCert(ctx, &ypb.Empty{})
		}, "failed to download mitm cert")),

		WithTool(mcp.NewTool("start_mitm_v2",
			mcp.WithDescription("Start MITM v2 HTTPS proxy in background (status:started). Typical flow: download_mitm_cert → set system/browser proxy to host:port → set_mitm_filter / set_current_rules"),
			mcp.WithString("host", mcp.Description("Listen address, usually 127.0.0.1")),
			mcp.WithNumber("port", mcp.Description("Listen port for HTTP/HTTPS proxy"), mcp.Required()),
			mcp.WithString("downstreamProxy", mcp.Description("Upstream proxy URL chained after MITM, e.g. http://127.0.0.1:7890")),
			mcp.WithBool("enableHttp2", mcp.Description("Terminate and forward HTTP/2")),
			mcp.WithBool("filterWebsocket", mcp.Description("Skip websocket upgrade traffic")),
			mcp.WithNumber("maxContentLength", mcp.Description("Max request/response body bytes to capture")),
			mcp.WithBool("disableSystemProxy", mcp.Description("Do not read OS HTTP_PROXY/HTTPS_PROXY for upstream")),
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
