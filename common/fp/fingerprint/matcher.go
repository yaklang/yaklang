package fingerprint

import (
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/fp/fingerprint/rule"
	"regexp"
)

var (
	invalidParamError = errors.New("invalid param")
)

type MatchFun func(data []byte) (bool, error)
type Matcher struct {
	regexpCache map[string]*regexp.Regexp
	ErrorHandle func(error)
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
func (m *Matcher) Match(data []byte) []*rule.FingerprintInfo {
	var result []*rule.FingerprintInfo
	for _, r := range m.rules {
		f, err := m.LoadMethod(r.Method, r.MatchParam)
		if err != nil {
			m.ErrorHandle(err)
			continue
		}
		ok, err := f(data)
		if err != nil {
			m.ErrorHandle(err)
			continue
		}
		if ok {
			result = append(result, r.MatchParam.Info)
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
