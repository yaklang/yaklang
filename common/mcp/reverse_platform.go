package mcp

import (
	"context"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/cybertunnel"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var bridgeConnectToolOptions = []mcp.ToolOption{
	mcp.WithString("addr", mcp.Description("Yak Bridge address host:port; auto-filled from get_bridge_log_server if omitted")),
	mcp.WithString("secret", mcp.Description("Yak Bridge secret; auto-filled from get_bridge_log_server if omitted")),
}

var dnsLogBridgeToolOptions = []mcp.ToolOption{
	mcp.WithString("dnsLogAddr", mcp.Description("Remote Yak Bridge DNSLog address host:port; auto-filled from get_bridge_log_server when omitted on remote path")),
	mcp.WithString("dnsLogAddrSecret", mcp.Description("Remote Bridge secret for require_dnslog_domain only; auto-filled from get_bridge_log_server when omitted")),
	mcp.WithString("dnsMode", mcp.Description(`Broker/platform name, e.g. "dnslog.cn", or "*" for random; must match between require and query when useLocal is true`)),
	mcp.WithBool("useLocal", mcp.Description("Use in-process third-party DNSLog broker (e.g. dnslog.cn), NOT start_facades embedded DNS; auto-fallback to Bridge remote on broker failure")),
	mcp.WithBool("useRemote", mcp.Description("Hint only: remote Bridge is used when useLocal is false (default); gRPC routes by useLocal")),
}

func init() {
	AddGlobalToolSet("reverse_platform",
		WithTool(mcp.NewTool("get_global_reverse_server",
			mcp.WithDescription("Get Yak global reverse addresses. ConfiguredLocalAddr matches Yakit UI '本地反连 IP' (may be empty). EffectiveLocalReverseAddr/LocalReverseListener are runtime callback addresses (default 127.0.0.1 when bridge active). PublicReverse* is active Yak Bridge tunnel only"),
		), handleGetGlobalReverseServer),

		WithTool(mcp.NewTool("require_dnslog_domain",
			append([]mcp.ToolOption{
				mcp.WithDescription("Request DNSLog subdomain and token for OOB detection. Default: remote Yak Bridge. Omit useLocal for Bridge. Response includes Domain, Token, useLocal, useRemote, and fallbackToRemote if local broker failed"),
			}, dnsLogBridgeToolOptions...)...,
		), handleRequireDNSLogDomain),

		WithTool(mcp.NewTool("query_dnslog_by_token",
			append([]mcp.ToolOption{
				mcp.WithDescription("Query DNSLog hit events by token from require_dnslog_domain. Use the same path as require (same useLocal/dnsMode). Response: Events[], useLocal, useRemote, fallbackToRemote"),
				mcp.WithString("token", mcp.Description("Token returned by require_dnslog_domain"), mcp.Required()),
			}, dnsLogBridgeToolOptions...)...,
		), handleQueryDNSLogByToken),

		WithTool(mcp.NewTool("require_random_port_token",
			mcp.WithDescription("Request random high-port reverse token via configured Yak Bridge (get_bridge_log_server DNSLogAddr). Returns Token, Addr, Port. On failure, error indicates bridge port allocation timeout/unavailability (not DNSLog password); DNSLog may still work on the same host"),
		), handleRequireRandomPortToken),

		WithTool(mcp.NewTool("query_random_port_trigger",
			mcp.WithDescription("Query TCP reverse hit for a random-port token. Returns error if no connection yet (not an empty list). Auto-calls require_random_port_token when token omitted"),
			mcp.WithString("token", mcp.Description("Token from require_random_port_token")),
		), handleQueryRandomPortTrigger),

		WithTool(mcp.NewTool("get_bridge_log_server",
			mcp.WithDescription("Read persisted Yak Bridge address for DNSLog/random-port reverse (DNSLogAddr, DNSLogAddrSecret)"),
		), unaryEmptyToolHandler(func(ctx context.Context, s *MCPServer) (any, error) {
			return s.grpcClient.GetCurrentYakBridgeLogServer(ctx, &ypb.Empty{})
		}, "failed to get bridge log server")),

		WithTool(mcp.NewTool("set_bridge_log_server",
			mcp.WithDescription("Persist Yak Bridge DNSLog/reverse server config used by remote require/query and random-port tools"),
			mcp.WithString("dnsLogAddr", mcp.Description("Bridge server address host:port, e.g. ns1.example.com:64333")),
			mcp.WithString("dnsLogAddrSecret", mcp.Description("Bridge authentication secret")),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.YakDNSLogBridgeAddr) (any, error) {
			return s.grpcClient.SetYakBridgeLogServer(ctx, req)
		}, "failed to set bridge log server")),

		WithTool(mcp.NewTool("start_facades",
			mcp.WithDescription("Start local Facades (embedded DNSLog + RMI/HTTP/HTTPS) in background; returns status:started immediately. NOT wired to require_dnslog_domain/query_dnslog_by_token unless DNSLogRemotePort mirrors to Bridge. RMI/HTTP hits may create risks; DNS hits are stream-only"),
			mcp.WithString("localFacadeHost", mcp.Description("Local bind host for RMI/HTTP/HTTPS facades, e.g. 127.0.0.1 or 0.0.0.0")),
			mcp.WithNumber("localFacadePort", mcp.Description("Local facades listen port for RMI/HTTP/HTTPS; 0 disables")),
			mcp.WithBool("enableDNSLogServer", mcp.Description("Start embedded local DNS server (separate from dnslog.cn broker path)")),
			mcp.WithNumber("dnsLogLocalPort", mcp.Description("Local UDP DNS listen port when enableDNSLogServer is true")),
			mcp.WithNumber("dnsLogRemotePort", mcp.Description("Mirror local DNS UDP to this port on Yak Bridge; required to query DNS hits remotely")),
			mcp.WithNumber("facadeRemotePort", mcp.Description("Mirror local RMI/HTTP TCP to this port on Yak Bridge")),
			mcp.WithString("externalDomain", mcp.Description("Root domain for embedded DNSLog A records, must resolve to Bridge external IP when using remote mirror")),
			mcp.WithBool("verify", mcp.Description("Verify externalDomain resolves to Bridge exit IP before start")),
			mcp.WithStruct("connectParam", []mcp.PropertyOption{
				mcp.Description("Yak Bridge connection (addr, secret); required for remote port mirror and DNS external IP lookup"),
			}, bridgeConnectToolOptions...),
		), handleStartFacades),
	)
}

func handleGetGlobalReverseServer(s *MCPServer) server.ToolHandlerFunc {
	return func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		rsp, err := s.grpcClient.GetGlobalReverseServer(ctx, &ypb.Empty{})
		if err != nil {
			return nil, utils.Wrap(err, "failed to get global reverse server")
		}
		return NewCommonCallToolResult(globalReverseServerResult(rsp))
	}
}

func globalReverseServerResult(rsp *ypb.GetGlobalReverseServerResponse) map[string]any {
	configured := rsp.GetLocalReverseAddr()
	effectiveHost := consts.GetEffectiveLocalReverseHost()
	effectiveAddr := consts.GetEffectiveLocalReverseAddr()

	out := map[string]any{
		"PublicReverseIP":           rsp.GetPublicReverseIP(),
		"PublicReversePort":         rsp.GetPublicReversePort(),
		"ConfiguredLocalAddr":       configured,
		"EffectiveLocalReverseAddr": effectiveHost,
		"LocalReversePort":          rsp.GetLocalReversePort(),
		// Legacy fields: LocalReverseAddr is the UI-configured value (not runtime default).
		"LocalReverseAddr": rsp.GetLocalReverseAddr(),
	}
	if effectiveAddr != "" {
		out["LocalReverseListener"] = effectiveAddr
	} else if effectiveHost != "" && rsp.GetLocalReversePort() > 0 {
		out["LocalReverseListener"] = utils.HostPort(effectiveHost, int(rsp.GetLocalReversePort()))
	}
	if configured == "" && effectiveHost != "" {
		out["note"] = "ConfiguredLocalAddr is empty (matches Yakit UI); EffectiveLocalReverseAddr is the runtime callback IP"
	}
	return out
}

func handleRequireRandomPortToken(s *MCPServer) server.ToolHandlerFunc {
	return func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		rsp, err := s.grpcClient.RequireRandomPortToken(ctx, &ypb.Empty{})
		if err != nil {
			return nil, utils.Wrap(err, "failed to require random port token")
		}
		out := map[string]any{
			"Token": rsp.GetToken(),
			"Addr":  rsp.GetAddr(),
			"Port":  rsp.GetPort(),
		}
		if cfg, cfgErr := s.grpcClient.GetCurrentYakBridgeLogServer(ctx, &ypb.Empty{}); cfgErr == nil && cfg != nil {
			out["bridgeDNSLogAddr"] = cfg.GetDNSLogAddr()
		}
		return NewCommonCallToolResult(out)
	}
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

func fillBridgeFromConfig(ctx context.Context, s *MCPServer, addr, secret string) (string, string) {
	if addr != "" && secret != "" {
		return addr, secret
	}
	cfg, err := s.grpcClient.GetCurrentYakBridgeLogServer(ctx, &ypb.Empty{})
	if err != nil || cfg == nil {
		return addr, secret
	}
	if addr == "" {
		addr = cfg.GetDNSLogAddr()
	}
	if secret == "" {
		secret = cfg.GetDNSLogAddrSecret()
	}
	return addr, secret
}

func dnsLogDomainResult(rsp *ypb.DNSLogRootDomain, useLocal, useRemote, fallbackToRemote bool, dnsMode string) map[string]any {
	out := map[string]any{"Domain": rsp.GetDomain(), "Token": rsp.GetToken(), "useLocal": useLocal, "useRemote": useRemote}
	if fallbackToRemote {
		out["fallbackToRemote"] = true
	}
	if dnsMode != "" {
		out["dnsMode"] = dnsMode
	}
	return out
}

func dnsLogQueryResult(rsp *ypb.QueryDNSLogByTokenResponse, useLocal, useRemote, fallbackToRemote bool) map[string]any {
	out := map[string]any{"Events": rsp.GetEvents(), "useLocal": useLocal, "useRemote": useRemote}
	if fallbackToRemote {
		out["fallbackToRemote"] = true
	}
	return out
}

func handleRequireDNSLogDomain(s *MCPServer) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var req ypb.YakDNSLogBridgeAddr
		if request.Params.Arguments != nil {
			if err := decodeYakRequest(request.Params.Arguments, &req); err != nil {
				return nil, err
			}
		}
		rsp, err := s.grpcClient.RequireDNSLogDomain(ctx, &req)
		if err != nil {
			return nil, utils.Wrap(err, "failed to require dnslog domain")
		}
		fallbackToRemote := false
		if req.GetUseLocal() {
			if issuedLocal, ok := cybertunnel.DNSLogTokenIssuedUseLocal(rsp.GetToken()); ok && !issuedLocal {
				fallbackToRemote = true
			}
		}
		return NewCommonCallToolResult(dnsLogDomainResult(rsp, req.GetUseLocal(), !req.GetUseLocal(), fallbackToRemote, req.GetDNSMode()))
	}
}

func handleQueryDNSLogByToken(s *MCPServer) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var req ypb.QueryDNSLogByTokenRequest
		if err := decodeYakRequest(request.Params.Arguments, &req); err != nil {
			return nil, err
		}
		fallbackToRemote := false
		if !req.GetUseLocal() {
			req.DNSLogAddr, _ = fillBridgeFromConfig(ctx, s, req.GetDNSLogAddr(), "")
		}
		if err := cybertunnel.ValidateDNSLogQueryPath(req.GetToken(), req.GetUseLocal(), req.GetDNSMode()); err != nil {
			return nil, utils.Wrap(err, "failed to query dnslog by token")
		}
		rsp, err := s.grpcClient.QueryDNSLogByToken(ctx, &req)
		if err != nil && req.GetUseLocal() && cybertunnel.ShouldFallbackFromLocalDNSLogBroker(err) {
			req.UseLocal, req.UseRemote = false, true
			req.DNSLogAddr, _ = fillBridgeFromConfig(ctx, s, req.GetDNSLogAddr(), "")
			rsp, err = s.grpcClient.QueryDNSLogByToken(ctx, &req)
			fallbackToRemote = err == nil
		}
		if err != nil {
			return nil, utils.Wrap(err, "failed to query dnslog by token")
		}
		useLocal := req.GetUseLocal()
		return NewCommonCallToolResult(dnsLogQueryResult(rsp, useLocal, !useLocal, fallbackToRemote))
	}
}

func handleQueryRandomPortTrigger(s *MCPServer) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var req ypb.QueryRandomPortTriggerRequest
		if request.Params.Arguments != nil {
			if err := decodeYakRequest(request.Params.Arguments, &req); err != nil {
				return nil, err
			}
		}
		if req.GetToken() == "" {
			tokenRsp, err := s.grpcClient.RequireRandomPortToken(ctx, &ypb.Empty{})
			if err != nil {
				return nil, utils.Wrap(err, "failed to require random port token for auto-token query")
			}
			req.Token = tokenRsp.GetToken()
		}
		rsp, err := s.grpcClient.QueryRandomPortTrigger(ctx, &req)
		if err != nil {
			return nil, utils.Wrap(err, "failed to query random port trigger")
		}
		return NewCommonCallToolResult(rsp)
	}
}
