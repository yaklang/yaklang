package main

import (
	"context"
	"net"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/cybertunnel/tpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type exampleReachabilityBridge struct {
	tpb.UnimplementedTunnelServer
	tpb.UnimplementedDNSLogServer
	calls atomic.Int32
}

func (s *exampleReachabilityBridge) CheckServerReachable(_ context.Context, req *tpb.CheckServerReachableRequest) (*tpb.CheckServerReachableResponse, error) {
	s.calls.Add(1)
	if req.Url != "example.com" || req.HttpCheck {
		return nil, status.Error(codes.InvalidArgument, "unexpected reachability example arguments")
	}
	return &tpb.CheckServerReachableResponse{Reachable: true}, nil
}

func (s *exampleReachabilityBridge) RequireDomain(_ context.Context, _ *tpb.RequireDomainParams) (*tpb.RequireDomainResponse, error) {
	s.calls.Add(1)
	return &tpb.RequireDomainResponse{Domain: "doc.example.test", Token: "doc-token", Mode: "test"}, nil
}

func (s *exampleReachabilityBridge) QueryExistedDNSLog(_ context.Context, req *tpb.QueryExistedDNSLogParams) (*tpb.QueryExistedDNSLogResponse, error) {
	s.calls.Add(1)
	if req.Token == "" {
		return nil, status.Error(codes.InvalidArgument, "missing DNSLog token")
	}
	return &tpb.QueryExistedDNSLogResponse{Events: []*tpb.DNSLogEvent{{Type: "A", Token: req.Token,
		Domain: "doc.example.test", RemoteAddr: "127.0.0.1:5353", RemoteIP: "127.0.0.1"}}}, nil
}

func (s *exampleReachabilityBridge) RequireHTTPRequestTrigger(_ context.Context, _ *tpb.RequireHTTPRequestTriggerParams) (*tpb.RequireHTTPRequestTriggerResponse, error) {
	s.calls.Add(1)
	return &tpb.RequireHTTPRequestTriggerResponse{PrimaryUrl: "http://doc.example.test", PrimaryHost: "doc.example.test", Token: "doc-token"}, nil
}

func (s *exampleReachabilityBridge) QueryExistedHTTPRequestTrigger(_ context.Context, req *tpb.QueryExistedHTTPRequestTriggerRequest) (*tpb.QueryExistedHTTPRequestTriggerResponse, error) {
	s.calls.Add(1)
	if req.Token == "" {
		return nil, status.Error(codes.InvalidArgument, "missing HTTPLog token")
	}
	return &tpb.QueryExistedHTTPRequestTriggerResponse{Notifications: []*tpb.HTTPRequestTriggerNotification{{Url: "http://127.0.0.1/doc", RemoteAddr: "127.0.0.1"}}}, nil
}

// Keep the example's real Yak -> risk -> gRPC call, replacing only the remote
// bridge. The public bridge's availability must not decide whether CI passes.
func setupRiskBridgeExample(t *testing.T, code string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	bridge := &exampleReachabilityBridge{}
	server := grpc.NewServer()
	tpb.RegisterTunnelServer(server, bridge)
	tpb.RegisterDNSLogServer(server, bridge)
	go server.Serve(listener)
	t.Cleanup(func() {
		server.Stop()
		listener.Close()
		if bridge.calls.Load() < 1 {
			t.Error("risk example must execute its real bridge RPC")
		}
		if strings.Contains(code, "risk.CheckServerReachable(") && bridge.calls.Load() != 1 {
			t.Errorf("reachability example made %d bridge calls, want 1", bridge.calls.Load())
		}
	})
	t.Setenv(consts.YAK_DNSLOG_BRIDGE_ADDR, listener.Addr().String())
	t.Setenv(consts.YAK_DNSLOG_BRIDGE_PASSWORD, "")
}
