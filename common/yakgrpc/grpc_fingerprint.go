package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) QueryFingerprint(context.Context, *ypb.QueryFingerprintRequest) (*ypb.QueryFingerprintResponse, error) {
	return nil, nil
}

func (s *Server) DeleteFingerprint(context.Context, *ypb.DeleteFingerprintRequest) (*ypb.Empty, error) {
	return nil, nil
}

func (s *Server) UpdateFingerprint(context.Context, *ypb.UpdateFingerprintRequest) (*ypb.Empty, error) {
	return nil, nil
}

func (s *Server) CreateFingerprint(context.Context, *ypb.CreateFingerprintRequest) (*ypb.Empty, error) {
	return nil, nil
}
