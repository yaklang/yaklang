package mcp

import (
	"context"
	"strings"

	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

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

func handleGetTunnelServerExternalIP(s *MCPServer) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var req ypb.GetTunnelServerExternalIPParams
		if request.Params.Arguments != nil {
			if err := decodeYakRequest(request.Params.Arguments, &req); err != nil {
				return nil, err
			}
		}
		req.Addr, req.Secret = fillBridgeFromConfig(ctx, s, req.GetAddr(), req.GetSecret())
		if req.GetAddr() == "" {
			return nil, utils.Error("bridge addr is required (set via set_bridge_log_server or pass addr)")
		}
		rsp, err := s.grpcClient.GetTunnelServerExternalIP(ctx, &req)
		if err != nil {
			return nil, utils.Wrap(err, "failed to get tunnel server external ip")
		}
		return NewCommonCallToolResult(rsp)
	}
}

func handleVerifyTunnelServerDomain(s *MCPServer) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var req ypb.VerifyTunnelServerDomainParams
		if err := decodeYakRequest(request.Params.Arguments, &req); err != nil {
			return nil, err
		}
		if req.ConnectParams == nil {
			req.ConnectParams = &ypb.GetTunnelServerExternalIPParams{}
		}
		req.ConnectParams.Addr, req.ConnectParams.Secret = fillBridgeFromConfig(
			ctx, s, req.ConnectParams.GetAddr(), req.ConnectParams.GetSecret(),
		)
		rsp, err := s.grpcClient.VerifyTunnelServerDomain(ctx, &req)
		if err != nil {
			return nil, utils.Wrap(err, "failed to verify tunnel server domain")
		}
		return NewCommonCallToolResult(rsp)
	}
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
		if err != nil && req.GetUseLocal() && strings.Contains(strings.ToLower(err.Error()), "dnsbroker") {
			req.DNSLogAddr, req.DNSLogAddrSecret = fillBridgeFromConfig(ctx, s, req.GetDNSLogAddr(), req.GetDNSLogAddrSecret())
			req.UseLocal = false
			req.UseRemote = true
			rsp, err = s.grpcClient.RequireDNSLogDomain(ctx, &req)
		}
		if err != nil {
			return nil, utils.Wrap(err, "failed to require dnslog domain")
		}
		return NewCommonCallToolResult(rsp)
	}
}

func handleQueryDNSLogByToken(s *MCPServer) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var req ypb.QueryDNSLogByTokenRequest
		if err := decodeYakRequest(request.Params.Arguments, &req); err != nil {
			return nil, err
		}
		if req.GetUseLocal() {
			req.DNSLogAddr, _ = fillBridgeFromConfig(ctx, s, req.GetDNSLogAddr(), "")
		}
		rsp, err := s.grpcClient.QueryDNSLogByToken(ctx, &req)
		if err != nil {
			return nil, utils.Wrap(err, "failed to query dnslog by token")
		}
		return NewCommonCallToolResult(rsp)
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
				return nil, utils.Wrap(err, "failed to require random port token")
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

func handleRegisterFacadesHTTP(s *MCPServer) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var req ypb.RegisterFacadesHTTPRequest
		if request.Params.Arguments != nil {
			if err := decodeYakRequest(request.Params.Arguments, &req); err != nil {
				return nil, err
			}
		}
		if len(req.GetHTTPResponse()) == 0 {
			req.HTTPResponse = []byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 2\r\n\r\nok")
		}
		rsp, err := s.grpcClient.RegisterFacadesHTTP(ctx, &req)
		if err != nil {
			return nil, utils.Wrap(err, "failed to register facades http")
		}
		return NewCommonCallToolResult(rsp)
	}
}

func handleStartFacadesWithYso(s *MCPServer) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var req ypb.StartFacadesWithYsoParams
		if err := decodeYakRequest(request.Params.Arguments, &req); err != nil {
			return nil, err
		}
		if req.GetReversePort() <= 0 {
			if rev, err := s.grpcClient.GetGlobalReverseServer(ctx, &ypb.Empty{}); err == nil && rev != nil {
				req.ReversePort = rev.GetLocalReversePort()
			}
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
