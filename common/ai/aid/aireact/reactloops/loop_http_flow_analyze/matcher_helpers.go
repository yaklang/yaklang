package loop_http_flow_analyze

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/httptpl"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type simpleMatcher struct {
	matcher *httptpl.YakMatcher
}

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

func newSimpleMatcherFromGRPC(m *ypb.HTTPResponseMatcher) *simpleMatcher {
	return &simpleMatcher{
		matcher: buildYakMatcherFromGRPC(m),
	}
}

func executeMatchers(matchers []*simpleMatcher, resp *httptpl.RespForMatch) (matched bool, err error) {
	var errs []error
	getErr := func() error {
		if len(errs) > 0 {
			return utils.JoinErrors(errs...)
		}
		return nil
	}
	for _, m := range matchers {
		matched, err := m.matcher.Execute(resp, nil)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if matched {
			return true, getErr()
		}
	}
	return false, getErr()
}
