package yakgrpc

import (
	"context"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"

	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) QuerySSAProject(ctx context.Context, req *ypb.QuerySSAProjectRequest) (*ypb.QuerySSAProjectResponse, error) {
	p, data, err := yakit.QuerySSAProject(consts.GetGormProfileDatabase(), req)
	if err != nil {
		return nil, err
	}
	rsp := &ypb.QuerySSAProjectResponse{
		Pagination: req.GetPagination(),
		Total:      int64(p.TotalRecord),
	}
	for _, d := range data {
		rsp.Projects = append(rsp.Projects, d.ToGRPCModel())
	}
	return rsp, nil
}

func (s *Server) CreateSSAProject(ctx context.Context, req *ypb.CreateSSAProjectRequest) (*ypb.CreateSSAProjectResponse, error) {
	if req == nil || req.Project == nil {
		return nil, utils.Errorf("create SSA project failed: request or project is nil")
	}

	project, err := yakit.CreateSSAProject(consts.GetGormProfileDatabase(), req.Project)
	if err != nil {
		return &ypb.CreateSSAProjectResponse{
			Message: &ypb.DbOperateMessage{
				TableName:    "ssa_projects",
				Operation:    DbOperationCreate,
				EffectRows:   0,
				ExtraMessage: err.Error(),
			},
		}, err
	}
	return &ypb.CreateSSAProjectResponse{
		Project: project.ToGRPCModel(),
		Message: &ypb.DbOperateMessage{
			TableName:    "ssa_projects",
			Operation:    DbOperationCreate,
			EffectRows:   1,
			ExtraMessage: "create SSA project success",
		},
	}, nil
}

func (s *Server) UpdateSSAProject(ctx context.Context, req *ypb.UpdateSSAProjectRequest) (*ypb.UpdateSSAProjectResponse, error) {
	if req == nil || req.Project == nil {
		return nil, utils.Errorf("update SSA project failed: request or project is nil")
	}

	project, err := yakit.UpdateSSAProject(consts.GetGormProfileDatabase(), req.Project)
	if err != nil {
		return &ypb.UpdateSSAProjectResponse{
			Message: &ypb.DbOperateMessage{
				TableName:    "ssa_projects",
				Operation:    DbOperationUpdate,
				EffectRows:   0,
				ExtraMessage: err.Error(),
			},
		}, err
	}
	return &ypb.UpdateSSAProjectResponse{
		Project: project.ToGRPCModel(),
		Message: &ypb.DbOperateMessage{
			TableName:    "ssa_projects",
			Operation:    DbOperationUpdate,
			EffectRows:   1,
			ExtraMessage: "update SSA project success",
		},
	}, nil
}

func (s *Server) DeleteSSAProject(ctx context.Context, req *ypb.DeleteSSAProjectRequest) (*ypb.DeleteSSAProjectResponse, error) {
	count, err := yakit.DeleteSSAProject(consts.GetGormProfileDatabase(), req)
	if err != nil {
		return &ypb.DeleteSSAProjectResponse{
			Message: &ypb.DbOperateMessage{
				TableName:    "ssa_projects",
				Operation:    DbOperationDelete,
				EffectRows:   0,
				ExtraMessage: err.Error(),
			},
		}, err
	}
	return &ypb.DeleteSSAProjectResponse{
		Message: &ypb.DbOperateMessage{
			TableName:  "ssa_projects",
			Operation:  DbOperationDelete,
			EffectRows: count,
		},
	}, nil
}
