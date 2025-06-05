package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) QueryAITask(ctx context.Context, req *ypb.AITaskQueryRequest) (*ypb.AITaskQueryResponse, error) {
	paging, data, err := yakit.QueryCoordinatorRuntime(s.GetProfileDatabase(), req.GetFilter(), req.GetPagination())
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
