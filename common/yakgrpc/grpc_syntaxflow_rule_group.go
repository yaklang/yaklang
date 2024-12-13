package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/consts"
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
	var groups []*ypb.SyntaxFlowGroup
	for _, group := range result {
		groups = append(groups, group.ToGRPCModel())
	}
	return &ypb.QuerySyntaxFlowRuleGroupResponse{Group: groups}, nil
}

func (s *Server) DeleteSyntaxFlowRuleGroup(ctx context.Context, req *ypb.DeleteSyntaxFlowRuleGroupRequest) (*ypb.DbOperateMessage, error) {
	msg := &ypb.DbOperateMessage{
		TableName: "syntax_flow_rule_group",
		Operation: DbOperationDelete,
	}
	if req.GetFilter() == nil {
		return nil, utils.Errorf("delete syntax flow rule group failed:filter is empty")
	}
	// 内置组默认不允许删除
	req.Filter.FilterGroupKind = "unBuildIn"
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
	var (
		errs  error
		count int64
	)
	db := s.GetProfileDatabase()
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

	if req.GetAddGroups() != nil {
		count, err = sfdb.BatchAddGroupsForRules(db, rules, req.GetAddGroups())
		msg.EffectRows += count
		if err != nil {
			errs = utils.JoinErrors(errs, err)
		}
	}
	if req.GetRemoveGroups() != nil {
		count, err = sfdb.BatchRemoveGroupsForRules(db, rules, req.GetRemoveGroups())
		msg.EffectRows += count
		if err != nil {
			errs = utils.JoinErrors(errs, err)
		}
	}
	return msg, errs
}

func (s *Server) UpdateSyntaxFlowRuleGroup(ctx context.Context, req *ypb.UpdateSyntaxFlowRuleGroupRequest) (*ypb.DbOperateMessage, error) {
	if req.GetOldGroupName() == "" || req.GetNewGroupName() == "" {
		return nil, utils.Errorf("update syntax flow rule group failed:group name is empty")
	}
	msg := &ypb.DbOperateMessage{
		TableName: "syntax_flow_rule_group",
		Operation: DbOperationUpdate,
	}
	err := sfdb.RenameGroup(consts.GetGormProfileDatabase(), req.GetOldGroupName(), req.GetNewGroupName())
	if err != nil {
		return nil, err
	} else {
		msg.EffectRows = 1
		return msg, nil
	}
}

func (s *Server) QuerySyntaxFlowSameGroup(ctx context.Context, req *ypb.QuerySyntaxFlowSameGroupRequest) (*ypb.QuerySyntaxFlowSameGroupResponse, error) {
	if req == nil || req.Filter == nil {
		return nil, utils.Errorf("query syntax flow same group failed:filter is empty")
	}
	groups, err := yakit.QuerySameGroupByRule(s.GetProfileDatabase(), req.GetFilter())
	if err != nil {
		return nil, utils.Errorf("query syntax flow same group failed:%s", err)
	}
	var result []*ypb.SyntaxFlowGroup
	for _, group := range groups {
		result = append(result, group.ToGRPCModel())
	}
	return &ypb.QuerySyntaxFlowSameGroupResponse{Group: result}, nil
}
