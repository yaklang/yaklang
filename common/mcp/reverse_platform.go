package mcp

import (
	"context"

	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var bridgeConnectToolOptions = []mcp.ToolOption{
	mcp.WithString("addr", mcp.Description("Yak Bridge address host:port; auto-filled from get_bridge_log_server if omitted")),
	mcp.WithString("secret", mcp.Description("Yak Bridge secret; auto-filled from get_bridge_log_server if omitted")),
}

var dnsLogBridgeToolOptions = []mcp.ToolOption{
	mcp.WithString("dnsLogAddr", mcp.Description("Remote DNSLog bridge address")),
	mcp.WithString("dnsLogAddrSecret", mcp.Description("Remote DNSLog bridge secret")),
	mcp.WithString("dnsMode", mcp.Description("DNSLog mode")),
	mcp.WithBool("useLocal", mcp.Description("Use local DNSLog server")),
	mcp.WithBool("useRemote", mcp.Description("Use remote DNSLog via bridge")),
}

func init() {
	AddGlobalToolSet("reverse_platform",
		WithTool(mcp.NewTool("get_global_reverse_server",
			mcp.WithDescription("Get global reverse server addresses (public IP/port and local listener)"),
		), unaryEmptyToolHandler(func(ctx context.Context, s *MCPServer) (any, error) {
			return s.grpcClient.GetGlobalReverseServer(ctx, &ypb.Empty{})
		}, "failed to get global reverse server")),

		WithTool(mcp.NewTool("require_dnslog_domain",
			append([]mcp.ToolOption{
				mcp.WithDescription("Request a DNSLog subdomain and token for OOB detection"),
			}, dnsLogBridgeToolOptions...)...,
		), handleRequireDNSLogDomain),

		WithTool(mcp.NewTool("query_dnslog_by_token",
			append([]mcp.ToolOption{
				mcp.WithDescription("Query DNSLog trigger events by token"),
				mcp.WithString("token", mcp.Description("DNSLog token from require_dnslog_domain"), mcp.Required()),
			}, dnsLogBridgeToolOptions...)...,
		), handleQueryDNSLogByToken),

		WithTool(mcp.NewTool("require_random_port_token",
			mcp.WithDescription("Request random-port reverse connection token and listen address"),
		), unaryEmptyToolHandler(func(ctx context.Context, s *MCPServer) (any, error) {
			return s.grpcClient.RequireRandomPortToken(ctx, &ypb.Empty{})
		}, "failed to require random port token")),

		WithTool(mcp.NewTool("query_random_port_trigger",
			mcp.WithDescription("Query random-port reverse trigger events; auto-requests token if omitted"),
			mcp.WithString("token", mcp.Description("Random port token from require_random_port_token")),
		), handleQueryRandomPortTrigger),

		WithTool(mcp.NewTool("get_bridge_log_server",
			mcp.WithDescription("Get configured Yak Bridge DNSLog/reverse server address"),
		), unaryEmptyToolHandler(func(ctx context.Context, s *MCPServer) (any, error) {
			return s.grpcClient.GetCurrentYakBridgeLogServer(ctx, &ypb.Empty{})
		}, "failed to get bridge log server")),

		WithTool(mcp.NewTool("set_bridge_log_server",
			mcp.WithDescription("Configure Yak Bridge DNSLog/reverse server"),
			mcp.WithString("dnsLogAddr", mcp.Description("Bridge server address host:port")),
			mcp.WithString("dnsLogAddrSecret", mcp.Description("Bridge server secret")),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.YakDNSLogBridgeAddr) (any, error) {
			return s.grpcClient.SetYakBridgeLogServer(ctx, req)
		}, "failed to set bridge log server")),

		WithTool(mcp.NewTool("start_facades",
			mcp.WithDescription("Start facades server with DNSLog/RMI/HTTP (background stream)"),
			mcp.WithString("localFacadeHost", mcp.Description("Local facades bind host")),
			mcp.WithNumber("localFacadePort", mcp.Description("Local facades port (RMI/HTTP/HTTPS)")),
			mcp.WithBool("enableDNSLogServer", mcp.Description("Enable embedded DNSLog server")),
			mcp.WithNumber("dnsLogLocalPort", mcp.Description("Local DNSLog port")),
			mcp.WithNumber("dnsLogRemotePort", mcp.Description("Remote DNSLog mirror port on bridge")),
			mcp.WithNumber("facadeRemotePort", mcp.Description("Remote facades mirror port on bridge")),
			mcp.WithString("externalDomain", mcp.Description("External DNS domain for DNSLog records")),
			mcp.WithBool("verify", mcp.Description("Verify tunnel domain before starting")),
			mcp.WithStruct("connectParam", []mcp.PropertyOption{
				mcp.Description("Bridge connection parameters"),
			}, bridgeConnectToolOptions...),
		), handleStartFacades),
	)
}

func handleStartFacades(s *MCPServer) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var req ypb.StartFacadesParams
		if err := decodeYakRequest(request.Params.Arguments, &req); err != nil {
			return nil, err
		}
		summary := map[string]any{
			"enableDNSLogServer": req.GetEnableDNSLogServer(),
			"dnsLogLocalPort":    req.GetDNSLogLocalPort(),
			"localFacadePort":    req.GetLocalFacadePort(),
			"externalDomain":     req.GetExternalDomain(),
		}
		return startBackgroundExecStream(s, "start_facades", summary, func(bgCtx context.Context) (execResultReceiver, error) {
			return s.grpcClient.StartFacades(bgCtx, &req)
		})
	}
}
