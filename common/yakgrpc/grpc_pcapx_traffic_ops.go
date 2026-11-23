package yakgrpc

import (
	"context"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

// QueryTrafficSession is kept only for wire compatibility.
func (s *Server) QueryTrafficSession(context.Context, *ypb.QueryTrafficSessionRequest) (*ypb.QueryTrafficSessionResponse, error) {
	return nil, grpcstatus.Error(codes.Unimplemented, "QueryTrafficSession is deprecated; use the pcapdb APIs")
}

// QueryTrafficPacket is kept only for wire compatibility.
func (s *Server) QueryTrafficPacket(context.Context, *ypb.QueryTrafficPacketRequest) (*ypb.QueryTrafficPacketResponse, error) {
	return nil, grpcstatus.Error(codes.Unimplemented, "QueryTrafficPacket is deprecated; use QueryPCAPPackets or GetPCAPPacketDetail")
}

// QueryTrafficTCPReassembled is kept only for wire compatibility.
func (s *Server) QueryTrafficTCPReassembled(context.Context, *ypb.QueryTrafficTCPReassembledRequest) (*ypb.QueryTrafficTCPReassembledResponse, error) {
	return nil, grpcstatus.Error(codes.Unimplemented, "QueryTrafficTCPReassembled is deprecated; use the pcapdb stream APIs")
}

// ParseTraffic is kept only for wire compatibility.
func (s *Server) ParseTraffic(context.Context, *ypb.ParseTrafficRequest) (*ypb.ParseTrafficResponse, error) {
	return nil, grpcstatus.Error(codes.Unimplemented, "ParseTraffic is deprecated; use GetPCAPPacketDetail")
}
