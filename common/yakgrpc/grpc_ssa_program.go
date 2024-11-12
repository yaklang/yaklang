package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) QuerySsaPrograms(ctx context.Context, req *ypb.QuerySsaProgramRequest) (*ypb.QuerySsaProgramResponse, error) {
	var ypbPrograms []*ypb.SsaProgram
	pagine, programs, err := yakit.QuerySsaProgram(consts.GetGormProfileDatabase(), req)
	if err != nil {
		return nil, err
	}
	for _, program := range programs {
		ypbPrograms = append(ypbPrograms, program.ToGrpcProgram())
	}
	return &ypb.QuerySsaProgramResponse{
		Paging: &ypb.Paging{
			Page:  int64(pagine.Page),
			Limit: int64(pagine.Limit),
		},
		Programs: ypbPrograms,
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
