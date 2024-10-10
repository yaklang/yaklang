package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) QuerySyntaxFlowRuleGroup(ctx context.Context, req *ypb.QuerySyntaxFlowRuleGroupRequest) (*ypb.QuerySyntaxFlowRuleGroupResponse, error) {
	var rsps []*ypb.SyntaxFlowGroup
	result, err := yakit.QuerySyntaxFlowRuleGroup(s.GetProfileDatabase(), req)
	if err != nil {
		return nil, err
	}
	filterGroup := filter.NewFilter()
	for _, r := range result {
		if filterGroup.Exist(r.GroupName) {
			continue
		}
		rsp := &ypb.SyntaxFlowGroup{
			GroupName: r.GroupName,
			Count:     int32(r.Count),
		}
		rsps = append(rsps, rsp)
		filterGroup.Insert(r.GroupName)
	}
	return &ypb.QuerySyntaxFlowRuleGroupResponse{Group: rsps}, nil
}

