package fingerprint

import (
	"context"
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/fp/fingerprint/rule"
	"github.com/yaklang/yaklang/common/log"
	"regexp"
)

var (
	invalidParamError = errors.New("invalid param")
)

type MatchFun func(data []byte) (bool, error)
type Matcher struct {
	regexpCache map[string]*regexp.Regexp
	ErrorHandle func(error)
	Route       func(ctx context.Context, webPath string) ([]byte, error)
	rules       [][]*rule.OpCode
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
	for _, printRule := range rules {
		ops := printRule.ToOpCodes()
		if len(ops) != 0 {
			m.rules = append(m.rules, ops)
		}
	}
}
func (m *Matcher) Match(ctx context.Context, data []byte) []*rule.FingerprintInfo {
	var result []*rule.FingerprintInfo
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

func (m *Matcher) LoadMethod(name string, params *rule.MatchMethodParam) (MatchFun, error) {
	if v, ok := MethodGetterMap[name]; ok {
		return v(m, params)
	}
	return nil, fmt.Errorf("not found method: %v", name)
}
