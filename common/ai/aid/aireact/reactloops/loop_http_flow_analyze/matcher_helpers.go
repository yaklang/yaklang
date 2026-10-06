package loop_http_flow_analyze

import (
	"github.com/yaklang/yaklang/common/yak/httptpl"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func buildYakMatcherFromGRPC(m *ypb.HTTPResponseMatcher) *httptpl.YakMatcher {
	if m == nil {
		return nil
	}
	yakMatcher := &httptpl.YakMatcher{
		MatcherType:         m.GetMatcherType(),
		ExprType:            m.GetExprType(),
		Scope:               m.GetScope(),
		Condition:           m.GetCondition(),
		Group:               m.GetGroup(),
		GroupEncoding:       m.GetGroupEncoding(),
		Negative:            m.GetNegative(),
		SubMatcherCondition: m.GetSubMatcherCondition(),
	}
	for _, sub := range m.GetSubMatchers() {
		yakMatcher.SubMatchers = append(yakMatcher.SubMatchers, buildYakMatcherFromGRPC(sub))
	}
	return yakMatcher
}
