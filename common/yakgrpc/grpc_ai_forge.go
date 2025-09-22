package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) QueryAIForge(ctx context.Context, req *ypb.QueryAIForgeRequest) (*ypb.QueryAIForgeResponse, error) {
	paging, data, err := yakit.QueryAIForge(s.GetProfileDatabase(), req.GetFilter(), req.GetPagination())
	if err != nil {
		return nil, err
	}

	var res []*ypb.AIForge
	for _, r := range data {
		m := r.ToGRPC()
		if m == nil {
			log.Errorf("failed to convert schema to ypb grpc: %v", r)
		} else {
			res = append(res, m)
		}
	}

	return &ypb.QueryAIForgeResponse{
		Pagination: &ypb.Paging{
			Page:    int64(paging.Page),
			Limit:   int64(paging.Limit),
			OrderBy: req.GetPagination().GetOrderBy(),
			Order:   req.GetPagination().GetOrder(),
		},
		Total: int64(paging.TotalRecord),
		Data:  res,
	}, nil
}

func (s *Server) DeleteAIForge(ctx context.Context, req *ypb.AIForgeFilter) (*ypb.DbOperateMessage, error) {
	count, err := yakit.DeleteAIForge(s.GetProfileDatabase(), req)
	if err != nil {
		return nil, err
	}
	return &ypb.DbOperateMessage{
		TableName:  "ai_forge",
		Operation:  "delete",
		EffectRows: count,
	}, nil
}

func (s *Server) UpdateAIForge(ctx context.Context, req *ypb.AIForge) (*ypb.DbOperateMessage, error) {
	forge := schema.GRPC2AIForge(req)
	err := yakit.UpdateAIForge(s.GetProfileDatabase(), forge)
	if err != nil {
		return nil, err
	}
	updateMessage := &ypb.DbOperateMessage{
		TableName:  "ai_forge",
		Operation:  "update",
		EffectRows: int64(1),
	}
	return updateMessage, nil
}

func (s *Server) CreateAIForge(ctx context.Context, req *ypb.AIForge) (*ypb.DbOperateMessage, error) {
	forgeIns := schema.GRPC2AIForge(req)
	err := yakit.CreateAIForge(s.GetProfileDatabase(), forgeIns)
	if err != nil {
		return nil, err
	}
	return &ypb.DbOperateMessage{
		TableName:  "ai_forge",
		Operation:  "create",
		EffectRows: 1,
		CreateID:   int64(forgeIns.ID),
	}, nil
}

func (s *Server) GetAIForge(ctx context.Context, req *ypb.GetAIForgeRequest) (*ypb.AIForge, error) {
	var forge *schema.AIForge
	var err error
	if req.GetID() > 0 {
		forge, err = yakit.GetAIForgeByID(s.GetProfileDatabase(), req.GetID())
		if err != nil {
			return nil, err
		}
	} else {
		forge, err = yakit.GetAIForgeByName(s.GetProfileDatabase(), req.GetForgeName())
		if err != nil {
			return nil, err
		}
	}

	return forge.ToGRPC(), nil
}
