package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) QuerySFBuildInRuleUpdate(ctx context.Context, req *ypb.QuerySFBuildInRuleUpdateRequest) (*ypb.QuerySFBuildInRuleUpdateResponse, error) {
	needUpdate := yakit.Get(consts.EmbedSfBuildInRuleKey) != consts.ExistedSyntaxFlowEmbedFSHash
	if !needUpdate {
		return &ypb.QuerySFBuildInRuleUpdateResponse{NeedUpdate: needUpdate}, nil
	}
	rules := yakit.QueryBuildInRule(s.GetProfileDatabase())
	var state ypb.UpdateSFRuleState
	if len(rules) == 0 {
		state = ypb.UpdateSFRuleState_Rule_Empty
	} else {
		state = ypb.UpdateSFRuleState_Rule_To_Update
	}
	return &ypb.QuerySFBuildInRuleUpdateResponse{NeedUpdate: needUpdate, State: state}, nil
}

func (s *Server) UpdateSFBuildInRule(req *ypb.UpdateSFBuildInRuleRequest, stream ypb.Yak_UpdateSFBuildInRuleServer) error {
	defer yakit.Set(consts.EmbedSfBuildInRuleKey, consts.ExistedSyntaxFlowEmbedFSHash)
	notify := func(process float64, msg string) {
		stream.Send(&ypb.UpdateSFBuildInRuleResponse{
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
