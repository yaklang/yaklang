package main

import (
	"context"
	"net"
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
	calls atomic.Int32
}

func (s *exampleReachabilityBridge) CheckServerReachable(_ context.Context, req *tpb.CheckServerReachableRequest) (*tpb.CheckServerReachableResponse, error) {
	s.calls.Add(1)
	if req.Url != "example.com" || req.HttpCheck {
		return nil, status.Error(codes.InvalidArgument, "unexpected reachability example arguments")
	}
	return &tpb.CheckServerReachableResponse{Reachable: true}, nil
}

// Keep the example's real Yak -> risk -> gRPC call, replacing only the remote
// bridge. The public bridge's availability must not decide whether CI passes.
func setupReachabilityExample(t *testing.T) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	bridge := &exampleReachabilityBridge{}
	server := grpc.NewServer()
	tpb.RegisterTunnelServer(server, bridge)
	go server.Serve(listener)
	t.Cleanup(func() {
		server.Stop()
		listener.Close()
		if bridge.calls.Load() != 1 {
			t.Errorf("reachability example made %d bridge calls, want 1", bridge.calls.Load())
		}
	})
	t.Setenv(consts.YAK_DNSLOG_BRIDGE_ADDR, listener.Addr().String())
	t.Setenv(consts.YAK_DNSLOG_BRIDGE_PASSWORD, "")
}
