package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) QuerySyntaxFlowRuleGroup(ctx context.Context, req *ypb.QuerySyntaxFlowRuleGroupRequest) (*ypb.QuerySyntaxFlowRuleGroupResponse, error) {
	result, err := yakit.QuerySyntaxFlowRuleGroup(s.GetProfileDatabase(), req)
	if err != nil {
		return nil, err
	}
	return &ypb.QuerySyntaxFlowRuleGroupResponse{Group: result}, nil
}

func (s *Server) DeleteSyntaxFlowRuleGroup(ctx context.Context, req *ypb.DeleteSyntaxFlowRuleGroupRequest) (*ypb.DbOperateMessage, error) {
	msg := &ypb.DbOperateMessage{
		TableName: "syntax_flow_rule_group",
		Operation: DbOperationDelete,
	}
	count, err := yakit.DeleteSyntaxFlowRuleGroup(s.GetProfileDatabase(), req)
	msg.EffectRows = count
	return msg, err
}

func (s *Server) CreateSyntaxFlowRuleGroup(ctx context.Context, req *ypb.CreateSyntaxFlowRuleGroupRequest) (*ypb.DbOperateMessage, error) {
	msg := &ypb.DbOperateMessage{
		TableName:  "syntax_flow_rule_group",
		Operation:  DbOperationCreate,
		EffectRows: 1,
	}
	count, err := yakit.CreateSyntaxFlowRuleGroup(s.GetProfileDatabase(), req)
	msg.EffectRows = count
	return msg, err
}
