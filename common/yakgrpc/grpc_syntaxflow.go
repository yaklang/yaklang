package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) QuerySyntaxFlowRule(ctx context.Context, req *ypb.QuerySyntaxFlowRuleRequest) (*ypb.QuerySyntaxFlowRuleResponse, error) {
	p, data, err := yakit.QuerySyntaxFlowRule(s.GetProfileDatabase(), req)
	if err != nil {
		return nil, err
	}
	rsp := &ypb.QuerySyntaxFlowRuleResponse{
		Pagination: &ypb.Paging{
			Page:     int64(p.Page),
			Limit:    int64(p.Limit),
			OrderBy:  req.Pagination.OrderBy,
			Order:    req.Pagination.Order,
			RawOrder: req.Pagination.RawOrder,
		},
		DbMessage: &ypb.DbOperateMessage{
			TableName:  "syntax_flow_rule",
			Operation:  DbOperationQuery,
			EffectRows: int64(p.TotalRecord),
		},
	}
	for _, d := range data {
		rsp.Data = append(rsp.Data, d.ToGRPCModel())
	}
	return rsp, nil
}

func (s *Server) SaveSyntaxFlowRule(ctx context.Context, req *ypb.SaveSyntaxFlowRuleRequest) (*ypb.DbOperateMessage, error) {
	msg := &ypb.DbOperateMessage{
		TableName: "syntax_flow_rule",
		Operation: DbOperationCreateOrUpdate,
	}
	err := sfdb.SaveSyntaxFlowRule(req.GetRuleName(),req.GetLanguage(),req.GetContent(),req.GetTags()...)
	if err != nil {
		msg.EffectRows = 0
		return msg, err
	}
	msg.EffectRows = 1
	return msg, nil
}

func (s *Server) DeleteSyntaxFlowRule(ctx context.Context, req *ypb.DeleteSyntaxFlowRuleRequest) (*ypb.DbOperateMessage, error) {
	msg := &ypb.DbOperateMessage{
		TableName:    "syntax_flow_rule",
		Operation:    DbOperationDelete,
		EffectRows:   0,
		ExtraMessage: "",
	}
	count,err := yakit.DeleteSyntaxFlowRuleByFilter(s.GetProfileDatabase(), req.GetFilter())
	msg.EffectRows = count
	return msg, err
}
