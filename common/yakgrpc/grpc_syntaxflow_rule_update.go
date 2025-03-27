package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) CheckSyntaxFlowRuleUpdate(ctx context.Context, req *ypb.CheckSyntaxFlowRuleUpdateRequest) (*ypb.CheckSyntaxFlowRuleUpdateResponse, error) {
	needUpdate := yakit.Get(consts.EmbedSfBuildInRuleKey) != consts.ExistedSyntaxFlowEmbedFSHash
	if !needUpdate {
		return &ypb.CheckSyntaxFlowRuleUpdateResponse{NeedUpdate: false}, nil
	}
	rules := yakit.QueryBuildInRule(s.GetProfileDatabase())
	var state ypb.UpdateSyntaxFlowRuleState
	if len(rules) == 0 {
		state = ypb.UpdateSyntaxFlowRuleState_Rule_Empty
	} else {
		state = ypb.UpdateSyntaxFlowRuleState_Rule_To_Update
	}
	return &ypb.CheckSyntaxFlowRuleUpdateResponse{NeedUpdate: true, State: state}, nil
}

func (s *Server) ApplySyntaxFlowRuleUpdate(req *ypb.ApplySyntaxFlowRuleUpdateRequest, stream ypb.Yak_ApplySyntaxFlowRuleUpdateServer) error {
	defer yakit.Set(consts.EmbedSfBuildInRuleKey, consts.ExistedSyntaxFlowEmbedFSHash)
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
