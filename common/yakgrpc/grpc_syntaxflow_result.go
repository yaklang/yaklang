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

func (s *Server) DeleteSyntaxFlowResult(ctx context.Context, req *ypb.DeleteSyntaxFlowResultRequest) (*ypb.DeleteSyntaxFlowResultResponse, error) {
	db := ssadb.GetDB()
	if req.GetFilter() == nil && !req.GetDeleteAll() {
		return nil, utils.Errorf("parameter error: filter is required or set DeleteAll")
	}
	db = yakit.FilterSyntaxFlowResult(db, req.GetFilter())
	if req.GetDeleteContainRisk() {
		// if set DeleteContainRisk, delete all, and clear up risk
		// clear up risk
		resultID := make([]int64, 0)
		if err := db.Where("risk_count != 0").Pluck("id", &resultID).Error; err == nil {
			if err := yakit.DeleteSSARiskBySFResult(ssadb.GetDB(), resultID); err != nil {
				return nil, utils.Errorf("delete risk failed: %s", err)
			}
		}
	} else {
		// if not set, only delete risk_count = 0
		db = db.Where("risk_count = 0")
	}
	// delete
	effectRows, err := ssadb.DetleteResultByDB(db)
	if err != nil {
		return nil, utils.Errorf("delete failed: %s", err)
	}

	return &ypb.DeleteSyntaxFlowResultResponse{
		Message: &ypb.DbOperateMessage{
			TableName:    "audit_result",
			Operation:    "delete",
			EffectRows:   effectRows,
			ExtraMessage: "",
		},
	}, nil
}
