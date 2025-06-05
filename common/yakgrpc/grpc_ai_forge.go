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
	err := yakit.CreateAIForge(s.GetProfileDatabase(), schema.GRPC2AIForge(req))
	if err != nil {
		return nil, err
	}
	return &ypb.DbOperateMessage{
		TableName:  "ai_forge",
		Operation:  "create",
		EffectRows: 1,
	}, nil
}
