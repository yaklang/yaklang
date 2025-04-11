package yakgrpc

import (
	"context"

	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) CheckSyntaxFlowRuleUpdate(ctx context.Context, req *ypb.CheckSyntaxFlowRuleUpdateRequest) (*ypb.CheckSyntaxFlowRuleUpdateResponse, error) {
	needUpdate := sfbuildin.CheckEmbedRule()
	if !needUpdate {
		return &ypb.CheckSyntaxFlowRuleUpdateResponse{NeedUpdate: false}, nil
	}
	rules := yakit.QueryBuildInRule(s.GetProfileDatabase())
	state := ""
	if len(rules) == 0 {
		state = "empty"
	} else {
		state = "to_update"
	}
	return &ypb.CheckSyntaxFlowRuleUpdateResponse{NeedUpdate: true, State: state}, nil
}

func (s *Server) ApplySyntaxFlowRuleUpdate(req *ypb.ApplySyntaxFlowRuleUpdateRequest, stream ypb.Yak_ApplySyntaxFlowRuleUpdateServer) error {
	notify := func(process float64, msg string) {
		stream.Send(&ypb.ApplySyntaxFlowRuleUpdateResponse{
			Percent: process,
			Message: msg,
		})
	}
	err := sfbuildin.SyncEmbedRule(notify)
	if err != nil {
		return err
	}
	return nil
}
