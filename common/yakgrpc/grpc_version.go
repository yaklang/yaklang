package yakgrpc

import (
	"context"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) Version(ctx context.Context, _ *ypb.Empty) (*ypb.VersionResponse, error) {
	return &ypb.VersionResponse{Version: consts.GetPalmVersion()}, nil
}

func (s *Server) YakVersionAtLeast(ctx context.Context, req *ypb.YakVersionAtLeastRequest) (*ypb.GeneralResponse, error) {
	version := req.GetYakVersion()
	if version == "" {
		version = consts.GetYakVersion()
	}
	ok := false
	if version == "dev" || version == "" {
		ok = true
	} else {
		ok = utils.VersionGreaterEqual(version, req.GetAtLeastVersion())
	}

	return &ypb.GeneralResponse{Ok: ok}, nil
}

func (s *Server) GetMachineID(ctx context.Context, _ *ypb.Empty) (*ypb.GetMachineIDResponse, error) {
	return &ypb.GetMachineIDResponse{MachineID: utils.GetMachineCode()}, nil
}
