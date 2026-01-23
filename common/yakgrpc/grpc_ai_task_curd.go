package yakgrpc

import (
	"context"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) QueryAITask(ctx context.Context, req *ypb.AITaskQueryRequest) (*ypb.AITaskQueryResponse, error) {
	paging, data, err := yakit.QueryAgentRuntime(s.GetProfileDatabase(), req.GetFilter(), req.GetPagination())
	if err != nil {
		return nil, err
	}

	var res []*ypb.AITask
	for _, r := range data {
		m := r.ToGRPC()
		if m == nil {
			log.Errorf("failed to convert schema to ypb: %v", r)
		} else {
			res = append(res, m)
		}
	}

	return &ypb.AITaskQueryResponse{
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

func (s *Server) DeleteAITask(ctx context.Context, req *ypb.AITaskDeleteRequest) (*ypb.DbOperateMessage, error) {
	filter := (*ypb.AITaskFilter)(nil)
	if req != nil {
		filter = req.GetFilter()
	}
	db := s.GetProjectDatabase()

	// Fast clear (drop+recreate) for the "clear all" case.
	if filter == nil || (len(filter.GetName()) == 0 && len(filter.GetKeyword()) == 0 && len(filter.GetForgeName()) == 0 && len(filter.GetCoordinatorId()) == 0) {
		if err := yakit.DropAIAgentRuntimeTable(db); err != nil {
			return nil, err
		}
		return &ypb.DbOperateMessage{
			TableName:    "ai_agent_runtimes",
			Operation:    "clear",
			EffectRows:   0,
			ExtraMessage: "fast cleared by drop+automigrate",
		}, nil
	}

	effectCount, err := yakit.DeleteAgentRuntime(db, filter)
	if err != nil {
		return nil, err
	}
	return &ypb.DbOperateMessage{
		TableName:  "ai_agent_runtimes",
		Operation:  "delete",
		EffectRows: effectCount,
	}, nil
}
