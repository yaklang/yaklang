package yakgrpc

import (
	"context"
	"yaklang.io/yaklang/common/yakgrpc/yakit"
	"yaklang.io/yaklang/common/yakgrpc/ypb"
)

func (s *Server) QueryExecHistory(ctx context.Context, req *ypb.ExecHistoryRequest) (*ypb.ExecHistoryRecordResponse, error) {
	paging, data, err := yakit.QueryExecHistory(s.GetProjectDatabase(), req)
	if err != nil {
		return nil, err
	}

	var res []*ypb.ExecHistoryRecord
	for _, r := range data {
		res = append(res, r.ToGRPCModel())
	}
	return &ypb.ExecHistoryRecordResponse{
		Data: res,
		Pagination: &ypb.Paging{
			Page:    int64(paging.Page),
			Limit:   int64(paging.Limit),
			OrderBy: req.GetPagination().GetOrderBy(),
			Order:   req.GetPagination().GetOrder(),
		},
		Total: int64(paging.TotalRecord),
	}, nil
}
