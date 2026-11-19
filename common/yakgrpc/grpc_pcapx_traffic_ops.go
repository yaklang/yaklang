package yakgrpc

import (
	"context"
	"strings"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

// QueryTrafficPacket is a deprecated compatibility shell for callers that have
// already migrated to pcapdb DatasetID. It never reads the legacy project DB.
func (s *Server) QueryTrafficPacket(ctx context.Context, req *ypb.QueryTrafficPacketRequest) (*ypb.QueryTrafficPacketResponse, error) {
	if strings.TrimSpace(req.GetDatasetID()) == "" {
		return nil, grpcstatus.Error(codes.InvalidArgument, "QueryTrafficPacket is deprecated; DatasetID is required, use QueryPCAPPackets or GetPCAPPacketDetail")
	}
	return s.queryPCAPTrafficPacket(ctx, req)
}

// QueryTrafficTCPReassembled is kept only for wire compatibility. New callers
// should use the pcapdb APIs; session and stream queries are a later phase.
func (s *Server) QueryTrafficTCPReassembled(context.Context, *ypb.QueryTrafficTCPReassembledRequest) (*ypb.QueryTrafficTCPReassembledResponse, error) {
	return nil, grpcstatus.Error(codes.Unimplemented, "QueryTrafficTCPReassembled is deprecated; legacy TCP reassembled storage is no longer served")
}

// QueryTrafficSession is kept only for wire compatibility. New callers should
// use QueryPCAPDatabase until session queries are implemented in pcapdb.
func (s *Server) QueryTrafficSession(context.Context, *ypb.QueryTrafficSessionRequest) (*ypb.QueryTrafficSessionResponse, error) {
	return nil, grpcstatus.Error(codes.Unimplemented, "QueryTrafficSession is deprecated; legacy traffic session storage is no longer served")
}
