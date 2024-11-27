package yakgrpc

import (
	"context"
	"strings"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) QuerySyntaxFlowRule(ctx context.Context, req *ypb.QuerySyntaxFlowRuleRequest) (*ypb.QuerySyntaxFlowRuleResponse, error) {
	if req.Pagination == nil {
		req.Pagination = &ypb.Paging{
			Page:    1,
			Limit:   30,
			OrderBy: "updated_at",
			Order:   "desc",
		}
	}
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
		Total: uint64(p.TotalRecord),
		DbMessage: &ypb.DbOperateMessage{
			TableName: "syntax_flow_rule",
			Operation: DbOperationQuery,
		},
	}
	for _, d := range data {
		rsp.Rule = append(rsp.Rule, d.ToGRPCModel())
	}
	return rsp, nil
}

func (s *Server) CreateSyntaxFlowRule(ctx context.Context, req *ypb.CreateSyntaxFlowRuleRequest) (*ypb.DbOperateMessage, error) {
	msg := &ypb.DbOperateMessage{
		TableName:  "syntax_flow_rule",
		Operation:  DbOperationCreate,
		EffectRows: 1,
	}
	rule, err := ParseSyntaxFlowInput(req.GetSyntaxFlowInput())
	if err != nil {
		return nil, err
	}
	err = yakit.CreateSyntaxFlowRule(s.GetProfileDatabase(), rule)
	if err != nil {
		return nil, err
	}
	return msg, nil
}

func (s *Server) UpdateSyntaxFlowRule(ctx context.Context, req *ypb.UpdateSyntaxFlowRuleRequest) (*ypb.DbOperateMessage, error) {
	msg := &ypb.DbOperateMessage{
		TableName:  "syntax_flow_rule",
		Operation:  DbOperationCreateOrUpdate,
		EffectRows: 1,
	}
	rule, err := ParseSyntaxFlowInput(req.GetSyntaxFlowInput())
	if err != nil {
		return nil, err
	}
	err = yakit.UpdateSyntaxFlowRule(s.GetProfileDatabase(), rule)
	if err != nil {
		return nil, err
	}
	return msg, nil
}

func (s *Server) DeleteSyntaxFlowRule(ctx context.Context, req *ypb.DeleteSyntaxFlowRuleRequest) (*ypb.DbOperateMessage, error) {
	msg := &ypb.DbOperateMessage{
		TableName:    "syntax_flow_rule",
		Operation:    DbOperationDelete,
		EffectRows:   0,
		ExtraMessage: "",
	}
	count, err := yakit.DeleteSyntaxFlowRule(s.GetProfileDatabase(), req)
	msg.EffectRows = count
	return msg, err
}

func ParseSyntaxFlowInput(ruleInput *ypb.SyntaxFlowRuleInput) (*schema.SyntaxFlowRule, error) {
	language, err := sfdb.CheckSyntaxFlowLanguage(ruleInput.Language)
	if err != nil {
		return nil, err
	}
	rule, _ := sfdb.CheckSyntaxFlowRuleContent(ruleInput.Content)
	rule.Language = string(language)
	rule.RuleName = ruleInput.RuleName
	rule.Tag = strings.Join(ruleInput.Tags, "|")
	rule.Title = ruleInput.RuleName
	return rule, nil
}
