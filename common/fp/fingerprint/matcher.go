package fingerprint

import (
	"context"
	"errors"
	"regexp"
	"sync"

	"github.com/yaklang/yaklang/common/fp/fingerprint/rule"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// ErrActiveProbeDisabled lets passive scans skip rules that require another request.
var ErrActiveProbeDisabled = errors.New("active fingerprint probe disabled")

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
	getter = cachingGetter(getter)
	if paths := rootRegexpPaths(rules); len(paths) > 0 {
		pages := make(map[string][]byte, len(paths))
		failed := make(map[string]bool, len(paths))
		for _, path := range paths {
			res, err := getter(path)
			if err != nil || res == nil {
				failed[path] = true
				continue
			}
			pages[path] = res.Data
		}
		rules = filterRootWebRules(rules, pages, failed)
	}
	var (
		result []*schema.CPE
		mu     sync.Mutex
	)
	swg := utils.NewSizedWaitGroup(concurrency)
	for _, r := range rules {
		r := r
		err := swg.AddWithContext(ctx)
		if err != nil {
			// Join rules already running before returning their results.
			break
		}
		go func(r *rule.FingerPrintRule) {
			defer swg.Done()
			info, err := rule.Execute(getter, r)
			if err != nil {
				if !errors.Is(err, ErrActiveProbeDisabled) {
					log.Errorf("execute rule failed: %v", err)
				}
				return
			}
			if info != nil {
				mu.Lock()
				result = append(result, info)
				mu.Unlock()
			}
		}(r)
	}
	swg.Wait()
	return result
}

func (m *Matcher) Match(ctx context.Context, data []byte, rules []*rule.FingerPrintRule) []*schema.CPE {
	return m.MatchResource(ctx, 1, rules, func(path string) (*rule.MatchResource, error) {
		if path == "" || path == "/" {
			return rule.NewHttpResource(data), nil
		}
		data, err := m.Route(ctx, path)
		if err != nil {
			return nil, err
		}
		return rule.NewHttpResource(data), nil
	})
}
