package yakgrpc

import (
	"context"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) QuerySyntaxFlowResult(ctx context.Context, req *ypb.QuerySyntaxFlowResultRequest) (*ypb.QuerySyntaxFlowResultResponse, error) {

	db := ssadb.GetDB()
	db = yakit.FilterSyntaxFlowResult(db, req.GetFilter())

	paging := req.GetPagination()
	if paging == nil {
		paging = &ypb.Paging{
			Page:    1,
			Limit:   30,
			OrderBy: "updated_at",
			Order:   "desc",
		}
	}
	var results []*ssadb.AuditResult
	db = bizhelper.OrderByPaging(db, paging)
	p, db := bizhelper.Paging(db, int(paging.Page), int(paging.Limit), &results)
	if db.Error != nil {
		return nil, utils.Errorf("paging failed: %s", db.Error)
	}

	res := make([]*ypb.SyntaxFlowResult, 0, len(results))
	for _, r := range results {
		res = append(res, r.ToGRPCModel())
	}

	return &ypb.QuerySyntaxFlowResultResponse{
		Pagination: &ypb.Paging{
			Page:    int64(p.Page),
			Limit:   int64(p.Limit),
			OrderBy: paging.OrderBy,
			Order:   paging.Order,
		},
		Total: uint64(p.TotalRecord),
		DbMessage: &ypb.DbOperateMessage{
			TableName:  "audit_result",
			Operation:  DbOperationQuery,
			EffectRows: int64(p.TotalRecord),
		},
		Results: res,
	}, nil
}
