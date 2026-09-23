//go:build !irify_exclude

package yakgrpc

import (
	"context"

	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) CheckSyntaxFlowRuleUpdate(ctx context.Context, req *ypb.CheckSyntaxFlowRuleUpdateRequest) (*ypb.CheckSyntaxFlowRuleUpdateResponse, error) {
	needUpdate := sfbuildin.NeedSyncEmbedRule()
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
	// 前端手动更新按钮 → 使用强制同步。
	// ForceSyncEmbedRule 内部会先编译新快照，成功后再整体替换内置规则，
	// 所以这里不需要先显式删除：那样做会在同步失败时清空已安装的内置规则。
	err := sfbuildin.ForceSyncEmbedRule(notify)
	if err != nil {
		return err
	}
	return nil
}
