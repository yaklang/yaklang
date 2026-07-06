package mcp

import (
	"context"
	"errors"
	"io"

	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	AddGlobalToolSet("reverse_platform",
		WithTool(mcp.NewTool("get_global_reverse_server",
			mcp.WithDescription("Get global reverse connection server addresses (public and local)"),
		), unaryEmptyToolHandler(func(ctx context.Context, s *MCPServer) (any, error) {
			return s.grpcClient.GetGlobalReverseServer(ctx, &ypb.Empty{})
		}, "failed to get global reverse server")),

		WithTool(mcp.NewTool("available_local_addr",
			mcp.WithDescription("List available local network interfaces for reverse connections"),
		), unaryEmptyToolHandler(func(ctx context.Context, s *MCPServer) (any, error) {
			return s.grpcClient.AvailableLocalAddr(ctx, &ypb.Empty{})
		}, "failed to list local addresses")),

		WithTool(mcp.NewTool("get_tunnel_server_external_ip",
			mcp.WithDescription("Get external IP of the Yak Bridge tunnel server"),
			mcp.WithString("addr", mcp.Description("Bridge server address"), mcp.Required()),
			mcp.WithString("secret", mcp.Description("Bridge server secret"), mcp.Required()),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.GetTunnelServerExternalIPParams) (any, error) {
			return s.grpcClient.GetTunnelServerExternalIP(ctx, req)
		}, "failed to get tunnel server external ip")),

		WithTool(mcp.NewTool("verify_tunnel_server_domain",
			mcp.WithDescription("Verify that a domain resolves to the tunnel server external IP"),
			mcp.WithString("domain", mcp.Description("Domain to verify"), mcp.Required()),
			mcp.WithStruct("connectParams", []mcp.PropertyOption{
				mcp.Description("Bridge connection parameters"),
				mcp.Required(),
			},
				mcp.WithString("addr", mcp.Description("Bridge server address"), mcp.Required()),
				mcp.WithString("secret", mcp.Description("Bridge server secret"), mcp.Required()),
			),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.VerifyTunnelServerDomainParams) (any, error) {
			return s.grpcClient.VerifyTunnelServerDomain(ctx, req)
		}, "failed to verify tunnel server domain")),

		WithTool(mcp.NewTool("require_dnslog_domain",
			mcp.WithDescription("Request a DNSLog subdomain and token for out-of-band detection"),
			mcp.WithBool("useLocal", mcp.Description("Use local DNSLog server")),
			mcp.WithBool("useRemote", mcp.Description("Use remote DNSLog via bridge")),
			mcp.WithString("dnsLogAddr", mcp.Description("Remote DNSLog bridge address")),
			mcp.WithString("dnsLogAddrSecret", mcp.Description("Remote DNSLog bridge secret")),
			mcp.WithString("dnsMode", mcp.Description("DNS mode")),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.YakDNSLogBridgeAddr) (any, error) {
			return s.grpcClient.RequireDNSLogDomain(ctx, req)
		}, "failed to require dnslog domain")),

		WithTool(mcp.NewTool("query_dnslog_by_token",
			mcp.WithDescription("Query DNSLog events by token"),
			mcp.WithString("token", mcp.Description("DNSLog token"), mcp.Required()),
			mcp.WithBool("useLocal", mcp.Description("Use local DNSLog server")),
			mcp.WithBool("useRemote", mcp.Description("Use remote DNSLog via bridge")),
			mcp.WithString("dnsLogAddr", mcp.Description("Remote DNSLog bridge address")),
			mcp.WithString("dnsMode", mcp.Description("DNS mode")),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.QueryDNSLogByTokenRequest) (any, error) {
			return s.grpcClient.QueryDNSLogByToken(ctx, req)
		}, "failed to query dnslog by token")),

		WithTool(mcp.NewTool("require_random_port_token",
			mcp.WithDescription("Request a random port reverse connection token and address"),
		), unaryEmptyToolHandler(func(ctx context.Context, s *MCPServer) (any, error) {
			return s.grpcClient.RequireRandomPortToken(ctx, &ypb.Empty{})
		}, "failed to require random port token")),

		WithTool(mcp.NewTool("query_random_port_trigger",
			mcp.WithDescription("Query random port reverse connection trigger events by token"),
			mcp.WithString("token", mcp.Description("Random port token"), mcp.Required()),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.QueryRandomPortTriggerRequest) (any, error) {
			return s.grpcClient.QueryRandomPortTrigger(ctx, req)
		}, "failed to query random port trigger")),

		WithTool(mcp.NewTool("get_bridge_log_server",
			mcp.WithDescription("Get current Yak Bridge public reverse server configuration"),
		), unaryEmptyToolHandler(func(ctx context.Context, s *MCPServer) (any, error) {
			return s.grpcClient.GetCurrentYakBridgeLogServer(ctx, &ypb.Empty{})
		}, "failed to get bridge log server")),

		WithTool(mcp.NewTool("set_bridge_log_server",
			mcp.WithDescription("Set Yak Bridge public reverse server configuration"),
			mcp.WithString("dnsLogAddr", mcp.Description("Bridge server address")),
			mcp.WithString("dnsLogAddrSecret", mcp.Description("Bridge server secret")),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.YakDNSLogBridgeAddr) (any, error) {
			return s.grpcClient.SetYakBridgeLogServer(ctx, req)
		}, "failed to set bridge log server")),

		WithTool(mcp.NewTool("register_facades_http",
			mcp.WithDescription("Register an HTTP response on the local facades server"),
			mcp.WithNumber("httpFlowId", mcp.Description("HTTP flow ID to serve as response")),
			mcp.WithString("url", mcp.Description("Target URL path for the facades resource")),
			mcp.WithString("httpResponse", mcp.Description("Raw HTTP response content")),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.RegisterFacadesHTTPRequest) (any, error) {
			return s.grpcClient.RegisterFacadesHTTP(ctx, req)
		}, "failed to register facades http")),

		WithTool(mcp.NewTool("apply_class_to_facades",
			mcp.WithDescription("Generate a YSO class and apply it to the facades server"),
			mcp.WithString("token", mcp.Description("Reverse connection token"), mcp.Required()),
			mcp.WithStruct("generateClassParams", []mcp.PropertyOption{
				mcp.Description("YSO class generation parameters"),
				mcp.Required(),
			}),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.ApplyClassToFacadesParamsWithVerbose) (any, error) {
			return s.grpcClient.ApplyClassToFacades(ctx, req)
		}, "failed to apply class to facades")),

		WithTool(mcp.NewTool("config_global_reverse",
			mcp.WithDescription("Configure global reverse connection (starts in background, returns immediately)"),
			mcp.WithString("localAddr", mcp.Description("Local reverse listen address")),
			mcp.WithStruct("connectParams", []mcp.PropertyOption{
				mcp.Description("Bridge connection parameters for public reverse"),
			},
				mcp.WithString("addr", mcp.Description("Bridge server address")),
				mcp.WithString("secret", mcp.Description("Bridge server secret")),
			),
		), handleConfigGlobalReverse),

		WithTool(mcp.NewTool("start_facades",
			mcp.WithDescription("Start facades reverse server (DNSLog/RMI/HTTP, runs in background)"),
			mcp.WithBool("enableDNSLogServer", mcp.Description("Enable DNSLog server")),
			mcp.WithNumber("dnsLogLocalPort", mcp.Description("Local DNSLog port")),
			mcp.WithNumber("dnsLogRemotePort", mcp.Description("Remote DNSLog mirror port")),
			mcp.WithNumber("localFacadePort", mcp.Description("Local facades port (RMI/HTTP/HTTPS)")),
			mcp.WithNumber("facadeRemotePort", mcp.Description("Remote facades mirror port")),
			mcp.WithString("localFacadeHost", mcp.Description("Local facades host")),
			mcp.WithString("externalDomain", mcp.Description("External DNS domain for DNSLog")),
			mcp.WithBool("verify", mcp.Description("Verify tunnel domain before starting")),
			mcp.WithStruct("connectParam", []mcp.PropertyOption{
				mcp.Description("Bridge connection parameters"),
			},
				mcp.WithString("addr", mcp.Description("Bridge server address")),
				mcp.WithString("secret", mcp.Description("Bridge server secret")),
			),
		), handleStartFacades),

		WithTool(mcp.NewTool("start_facades_with_yso",
			mcp.WithDescription("Start facades with YSO object generation (runs in background)"),
			mcp.WithBool("isRemote", mcp.Description("Use remote bridge")),
			mcp.WithNumber("reversePort", mcp.Description("Reverse connection port")),
			mcp.WithString("reverseHost", mcp.Description("Reverse connection host")),
			mcp.WithString("token", mcp.Description("Reverse token")),
			mcp.WithStruct("bridgeParam", []mcp.PropertyOption{mcp.Description("Bridge parameters")},
				mcp.WithString("addr", mcp.Description("Bridge address")),
				mcp.WithString("secret", mcp.Description("Bridge secret")),
			),
			mcp.WithStruct("generateClassParams", []mcp.PropertyOption{
				mcp.Description("YSO class generation parameters"),
			}),
		), handleStartFacadesWithYso),
	)
}

func handleConfigGlobalReverse(s *MCPServer) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var req ypb.ConfigGlobalReverseParams
		if request.Params.Arguments != nil {
			if err := decodeYakRequest(request.Params.Arguments, &req); err != nil {
				return nil, err
			}
		}
		summary := map[string]any{
			"localAddr": req.GetLocalAddr(),
		}
		return startBackgroundConfigGlobalReverse(s, "config_global_reverse", summary, &req)
	}
}

func startBackgroundConfigGlobalReverse(s *MCPServer, name string, summary map[string]any, req *ypb.ConfigGlobalReverseParams) (*mcp.CallToolResult, error) {
	bgCtx := context.Background()
	stream, err := s.grpcClient.ConfigGlobalReverse(bgCtx, req)
	if err != nil {
		return nil, utils.Wrap(err, "failed to start config global reverse")
	}
	storeBackgroundStreamStatus(name, summary)
	go func() {
		for {
			_, err := stream.Recv()
			if err != nil {
				if !errors.Is(err, io.EOF) {
					appendBackgroundStreamLog(name, err.Error())
				}
				return
			}
		}
	}()
	return NewCommonCallToolResult(map[string]any{
		"status":  "started",
		"name":    name,
		"summary": summary,
	})
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

func handleStartFacadesWithYso(s *MCPServer) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var req ypb.StartFacadesWithYsoParams
		if err := decodeYakRequest(request.Params.Arguments, &req); err != nil {
			return nil, err
		}
		summary := map[string]any{
			"reverseHost": req.GetReverseHost(),
			"reversePort": req.GetReversePort(),
			"token":       req.GetToken(),
			"isRemote":    req.GetIsRemote(),
		}
		return startBackgroundExecStream(s, "start_facades_with_yso", summary, func(bgCtx context.Context) (execResultReceiver, error) {
			return s.grpcClient.StartFacadesWithYsoObject(bgCtx, &req)
		})
	}
}
