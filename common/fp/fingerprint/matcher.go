package fingerprint

import (
	"context"
	"github.com/yaklang/yaklang/common/fp/fingerprint/rule"
	"github.com/yaklang/yaklang/common/log"
	"regexp"
)

type MatchFun func(data []byte) (bool, error)
type Matcher struct {
	regexpCache map[string]*regexp.Regexp
	ErrorHandle func(error)
	Route       func(ctx context.Context, webPath string) ([]byte, error)
	rules       []*rule.FingerPrintRule
}

func NewMatcher(rules ...*rule.FingerPrintRule) *Matcher {
	matcher := &Matcher{
		ErrorHandle: func(err error) {},
		regexpCache: map[string]*regexp.Regexp{},
	}
	matcher.AddRules(rules)
	return matcher
}

func (m *Matcher) AddRules(rules []*rule.FingerPrintRule) {
	m.rules = append(m.rules, rules...)
}
func (m *Matcher) Match(ctx context.Context, data []byte) []*rule.CPE {
	var result []*rule.CPE
	cached := map[string][]byte{}
	for i, r := range m.rules {
		_ = i
		select {
		case <-ctx.Done():
			return result
		default:
		}
		info, err := rule.Execute(func(path string) (*rule.MatchResource, error) {
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
		}, r)
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
