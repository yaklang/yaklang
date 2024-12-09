package yakgrpc

import (
	"context"

	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils"
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

func (s *Server) CreateSyntaxFlowRuleGroup(ctx context.Context, req *ypb.CreateSyntaxFlowGroupRequest) (*ypb.DbOperateMessage, error) {
	msg := &ypb.DbOperateMessage{
		TableName: "syntax_flow_rule_group",
		Operation: DbOperationCreate,
	}
	if req.GetGroupName() == "" {
		return nil, utils.Errorf("add syntax flow rule group failed:group name is empty")
	}
	db := s.GetProfileDatabase()
	_, err := sfdb.CreateGroup(db, req.GetGroupName())
	if err != nil {
		return nil, err
	} else {
		msg.EffectRows = 1
		return msg, nil
	}
}

func (s *Server) UpdateSyntaxFlowRuleAndGroup(ctx context.Context, req *ypb.UpdateSyntaxFlowRuleAndGroupRequest) (*ypb.DbOperateMessage, error) {
	if req.GetFilter() == nil {
		return nil, utils.Errorf("update syntax flow rule group failed:filter is empty")
	}
	db := s.GetProfileDatabase()

	var errs error
	msg := &ypb.DbOperateMessage{
		TableName: "syntax_flow_rule_group",
		Operation: DbOperationUpdate,
	}
	rules, err := yakit.QuerySyntaxFlowRuleNames(db, req.GetFilter())
	if err != nil {
		return nil, err
	}
	if len(rules) == 0 {
		return nil, utils.Errorf("update syntax flow rule group failed:rule name is empty")
	}
	count, err := sfdb.BatchAddGroupsForRules(db, rules, req.GetAddGroups())
	msg.EffectRows += count
	if err != nil {
		errs = utils.JoinErrors(errs, err)
	}
	count, err = sfdb.BatchRemoveGroupsForRules(db, rules, req.GetRemoveGroups())
	msg.EffectRows += count
	if err != nil {
		errs = utils.JoinErrors(errs, err)
	}
	return msg, errs
}
