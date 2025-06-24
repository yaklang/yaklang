package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/syntaxflow/sfanalyzer"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) EvaluateSyntaxFlowRule(ctx context.Context, req *ypb.EvaluateSyntaxFlowRuleRequest) (*ypb.EvaluateSyntaxFlowRuleResponse, error) {
	ruleName := req.GetRuleName()
	var cancel func()
	ctx, cancel = context.WithCancel(ctx)
	defer cancel()

	var (
		ruleContent = req.GetRuleInput()
	)
	if ruleContent == "" {
		ins, err := yakit.GetSyntaxFlowRuleByName(s.GetProfileDatabase(), ruleName)
		if err != nil {
			return nil, err
		}
		ruleContent = ins.Content
	}
	sfAnalyzer := sfanalyzer.NewSyntaxFlowAnalyzer(ruleContent, ruleName)
	sfAnalyzeRes := sfAnalyzer.Analyze()
	return sfAnalyzeRes.GetResponse(), nil
}
