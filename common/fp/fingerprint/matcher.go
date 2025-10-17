package fingerprint

import (
	"context"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
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

func (m *Matcher) MatchResource(ctx context.Context, concurrency int, rules []*rule.FingerPrintRule, getter func(path string) (*rule.MatchResource, error)) []*schema.CPE {
	var result []*schema.CPE
	swg := utils.NewSizedWaitGroup(concurrency)
	for _, r := range rules {
		err := swg.AddWithContext(ctx)
		if err != nil {
			log.Errorf("failed to run rule %v: %v", r, err)
			return result
		}
		go func() {
			defer swg.Done()
			r := r
			info, err := rule.Execute(getter, r)
			if err != nil {
				log.Errorf("execute rule failed: %v", err)
			}
			if info != nil {
				result = append(result, info)
			}
		}()
	}
	swg.Wait()
	return result
}

func (m *Matcher) Match(ctx context.Context, data []byte, rules []*rule.FingerPrintRule) []*schema.CPE {
	return m.MatchResource(ctx, 1, rules, func(path string) (*rule.MatchResource, error) {
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
