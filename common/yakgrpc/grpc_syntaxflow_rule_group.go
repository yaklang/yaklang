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

