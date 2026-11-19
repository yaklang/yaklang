package yakgrpc

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/pcapx/pcapdb"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

func parsePCAPByteSource(sourceID string) (bool, uint, error) {
	kind, rawID, ok := strings.Cut(strings.TrimSpace(sourceID), ":")
	if !ok || rawID == "" || strings.Contains(rawID, ":") {
		return false, 0, grpcstatus.Error(codes.InvalidArgument, "SourceID must be frame:<positive-id> or message:<positive-id>")
	}
	id, err := strconv.ParseUint(rawID, 10, 64)
	if err != nil || id == 0 || uint64(uint(id)) != id {
		return false, 0, grpcstatus.Error(codes.InvalidArgument, "SourceID must contain a positive native record ID")
	}
	switch strings.ToLower(kind) {
	case "frame":
		return false, uint(id), nil
	case "message":
		return true, uint(id), nil
	default:
		return false, 0, grpcstatus.Error(codes.InvalidArgument, "SourceID supports only frame and message byte sources")
	}
}

func (s *Server) ReadPCAPBytes(ctx context.Context, req *ypb.ReadPCAPBytesRequest) (*ypb.ReadPCAPBytesResponse, error) {
	if err := validatePCAPDatasetID(req.GetDatasetID()); err != nil {
		return nil, err
	}
	protocol, id, err := parsePCAPByteSource(req.GetSourceID())
	if err != nil {
		return nil, err
	}
	if req.GetOffset() < 0 || req.GetLength() < 1 || req.GetLength() > pcapdb.MaxByteReadLength {
		return nil, grpcstatus.Error(codes.InvalidArgument,
			fmt.Sprintf("byte read requires Offset >= 0 and Length in 1..%d", pcapdb.MaxByteReadLength))
	}
	manager, err := s.getPCAPManager()
	if err != nil {
		return nil, pcapRPCError(err)
	}
	handle, err := manager.Acquire(ctx, req.GetDatasetID())
	if err != nil {
		return nil, pcapRPCError(err)
	}
	defer handle.Close()
	var result *pcapdb.DataRange
	if protocol {
		result, err = handle.ReadProtocolRange(id, req.GetOffset(), int(req.GetLength()))
	} else {
		result, err = handle.ReadPacketRange(id, req.GetOffset(), int(req.GetLength()))
	}
	if err != nil {
		return nil, pcapRPCError(err)
	}
	return &ypb.ReadPCAPBytesResponse{
		Data: result.Data, Offset: result.Offset, TotalLength: result.TotalLength, EOF: result.EOF,
	}, nil
}
