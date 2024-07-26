package fingerprint

import (
	"context"
	"regexp"

	"github.com/yaklang/yaklang/common/fp/fingerprint/rule"
	"github.com/yaklang/yaklang/common/log"
)

type (
	MatchFun func(data []byte) (bool, error)
	Matcher  struct {
		regexpCache map[string]*regexp.Regexp
		ErrorHandle func(error)
		Route       func(ctx context.Context, webPath string) ([]byte, error)
		// rules       []*rule.FingerPrintRule
	}
)

func NewMatcher() *Matcher {
	matcher := &Matcher{
		ErrorHandle: func(err error) {},
		regexpCache: map[string]*regexp.Regexp{},
	}
	return matcher
}

func (m *Matcher) MatchResource(ctx context.Context, rules []*rule.FingerPrintRule, getter func(path string) (*rule.MatchResource, error)) []*rule.CPE {
	var result []*rule.CPE
	for i, r := range rules {
		_ = i
		select {
		case <-ctx.Done():
			return result
		default:
		}
		info, err := rule.Execute(getter, r)
		if err != nil {
			log.Errorf("execute rule failed: %v", err)
			continue
		}
		if info != nil {
			result = append(result, info)
		}
	}
	return result
}

func (m *Matcher) Match(ctx context.Context, data []byte, rules []*rule.FingerPrintRule) []*rule.CPE {
	return m.MatchResource(ctx, rules, func(path string) (*rule.MatchResource, error) {
		cached := map[string][]byte{}
		if path == "" || path == "/" {
			return rule.NewHttpResource(data), nil
		}
		if v, ok := cached[path]; ok {
			return rule.NewHttpResource(v), nil
		}
		data, err := m.Route(ctx, path)
		if err != nil {
			return nil, err
		}
		cached[path] = data
		return rule.NewHttpResource(data), nil
	})
}
