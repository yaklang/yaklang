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
