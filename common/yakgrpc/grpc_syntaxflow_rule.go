package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) QuerySyntaxFlowRule(ctx context.Context, req *ypb.QuerySyntaxFlowRuleRequest) (*ypb.QuerySyntaxFlowRuleResponse, error) {
	p, data, err := yakit.QuerySyntaxFlowRule(s.GetProfileDatabase(), req)
	if err != nil {
		return nil, err
	}
	rsp := &ypb.QuerySyntaxFlowRuleResponse{
		Pagination: req.GetPagination(),
		Total:      uint64(p.TotalRecord),
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

func (s *Server) CreateSyntaxFlowRuleEx(ctx context.Context, req *ypb.CreateSyntaxFlowRuleRequest) (*ypb.CreateSyntaxFlowRuleResponse, error) {
	if req == nil || req.GetSyntaxFlowInput() == nil {
		return nil, utils.Error("create syntax flow rule failed: request is nil")
	}

	input := req.GetSyntaxFlowInput()
	rule, err := yakit.ParseSyntaxFlowInput(input)
	if err != nil {
		return nil, err
	}
	_, err = sfdb.CreateRuleWithDefaultGroup(rule, input.GetGroupNames()...)
	if err != nil {
		return nil, err
	}
	return &ypb.CreateSyntaxFlowRuleResponse{
		Rule: rule.ToGRPCModel(),
		Message: &ypb.DbOperateMessage{
			TableName:  "syntax_flow_rule",
			Operation:  DbOperationCreate,
			EffectRows: 1,
		},
	}, nil
}

func (s *Server) CreateSyntaxFlowRule(ctx context.Context, req *ypb.CreateSyntaxFlowRuleRequest) (*ypb.DbOperateMessage, error) {
	if ret, err := s.CreateSyntaxFlowRuleEx(ctx, req); err != nil {
		return nil, err
	} else {
		return ret.Message, nil
	}
}

func (s *Server) UpdateSyntaxFlowRuleEx(ctx context.Context, req *ypb.UpdateSyntaxFlowRuleRequest) (*ypb.UpdateSyntaxFlowRuleResponse, error) {
	if req == nil || req.SyntaxFlowInput == nil {
		return nil, utils.Error("update syntax flow rule failed: request is nil")
	}
	updatedRule, err := yakit.UpdateSyntaxFlowRule(s.GetProfileDatabase(), req.SyntaxFlowInput)
	if err != nil {
		return nil, err
	}
	return &ypb.UpdateSyntaxFlowRuleResponse{
		Message: &ypb.DbOperateMessage{
			TableName:  "syntax_flow_rule",
			Operation:  DbOperationCreateOrUpdate,
			EffectRows: 1,
		},
		Rule: updatedRule.ToGRPCModel(),
	}, nil
}

func (s *Server) UpdateSyntaxFlowRule(ctx context.Context, req *ypb.UpdateSyntaxFlowRuleRequest) (*ypb.DbOperateMessage, error) {
	if ret, err := s.UpdateSyntaxFlowRuleEx(ctx, req); err != nil {
		return nil, err
	} else {
		return ret.Message, nil
	}
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
