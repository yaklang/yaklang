package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/utils"
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

func (s *Server) CreateSyntaxFlowRuleEx(ctx context.Context, req *ypb.CreateSyntaxFlowRuleRequest) (*ypb.CreateSyntaxFlowRuleResponse, error) {
	if req == nil || req.GetSyntaxFlowInput() == nil {
		return nil, utils.Error("create syntax flow rule failed: request is nil")
	}

	input := req.GetSyntaxFlowInput()
	rule, err := ParseSyntaxFlowInput(input)
	if err != nil {
		return nil, err
	}
	_, err = sfdb.CreateRule(rule, input.GetGroupNames()...)
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
	//rule.Groups = sfdb.GetOrCreateGroups(consts.GetGormProfileDatabase(), ruleInput.GroupNames)
	rule.Description = ruleInput.Description
	return rule, nil
}
