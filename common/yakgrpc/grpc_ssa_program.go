package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) QuerySsaPrograms(ctx context.Context, req *ypb.QuerySsaProgramRequest) (*ypb.QuerySsaProgramResponse, error) {
	pagine, programs, err := yakit.QuerySsaProgram(consts.GetGormProfileDatabase(), req)
	if err != nil {
		return nil, err
	}
	for _, program := range programs {
		program.Recompile = program.GetEngineVersion() != consts.GetYakVersion()
	}
	return &ypb.QuerySsaProgramResponse{
		Paging: &ypb.Paging{
			Page:  int64(pagine.Page),
			Limit: int64(pagine.Limit),
		},
		Programs: programs,
		Total:    int64(pagine.TotalRecord),
	}, nil
}
func (s *Server) DeleteSsaPrograms(ctx context.Context, req *ypb.DeleteSsaProgramRequest) (*ypb.Empty, error) {
	if err := yakit.DeleteSsaProgram(consts.GetGormProjectDatabase(), req); err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) UpdateSsaProgram(ctx context.Context, prog *ypb.SsaProgram) (*ypb.Empty, error) {
	err := yakit.UpdateSsaProgram(consts.GetGormProfileDatabase(), schema.ToSchemaSsaProgram(prog))
	if err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}
