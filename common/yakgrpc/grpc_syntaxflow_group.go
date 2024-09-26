package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) QuerySyntaxFlowRuleGroup(ctx context.Context, req *ypb.QuerySyntaxFlowRuleGroupRequest) (*ypb.QuerySyntaxFlowRuleGroupResponse, error) {
	var rsp ypb.QuerySyntaxFlowRuleGroupResponse
	var groups []*ypb.SyntaxFlowRuleGroupNormalized
	var errs error
	filterGroup := filter.NewFilter()
	fields := []string{"language", "purpose", "severity"}

	for _, field := range fields {
		languageGroup, err := yakit.QuerySyntaxFlowRuleGroupByField(s.GetProfileDatabase(), field)
		errs = utils.JoinErrors(err, errs)
		groups = append(groups, languageGroup...)
	}

	for _, group := range groups {
		if filterGroup.Exist(group.GroupName) {
			continue
		}
		if group.GetGroupName() == "" {
			continue
		}
		if req.GetAll() {
			if req.GetIsBuiltinRule() != group.GetIsBuildInRule() {
				continue
			}
			rsp.Group = append(rsp.Group, group)
			filterGroup.Insert(group.GroupName)
		} else {
			if req.GetGroupName() != group.GroupName {
				continue
			}
			rsp.Group = append(rsp.Group, group)
			filterGroup.Insert(group.GroupName)
		}
	}
	return &rsp, errs
}


