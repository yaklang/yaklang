package regexp_utils

import (
	"errors"
	"sync"

	dlclark "github.com/dlclark/regexp2"
	"github.com/samber/lo"
)

// dlclarkWrapper 是 dlclark/regexp2 (.NET 语义) 的 RegWrapperInterface 实现.
//
// 用途: go-pcre2-lite (PCRE2) 对少数构造不支持, 典型是变长 lookbehind
// ((?<=a.*b)) —— PCRE2 在多数构建里要求 lookbehind 各分支定长 (code 125).
// dlclark/.NET 原生支持变长 lookbehind, 故作为 YakRegexpUtils 在 stdlib RE2 与
// PCRE2 都不可用时的终极兜底, 保证 yaklang 用户/规则里这类正则不致整体失效.
//
// 性能: dlclark 是纯 Go 回溯引擎, 比线性时间的 PCRE2 慢, 仅作少数 pattern 的兜底,
// 不影响热路径. 绝大多数 pattern 仍走 stdlib RE2 或 PCRE2.
type dlclarkWrapper struct {
	reg       *dlclark.Regexp
	regOnce   sync.Once
	regexpRaw string
	options   dlclark.RegexOptions
}

func newDlclarkWrapper(raw string, options dlclark.RegexOptions) *dlclarkWrapper {
	return &dlclarkWrapper{
		regexpRaw: raw,
		options:   options,
		regOnce:   sync.Once{},
	}
}

func (r *dlclarkWrapper) getReg() *dlclark.Regexp {
	r.regOnce.Do(func() {
		reg, err := dlclark.Compile(r.regexpRaw, r.options)
		if err != nil {
			return
		}
		// dlclark 默认 NoTimeout, 灾难性回溯会挂死; 设一个合理上限保护兜底路径.
		reg.MatchTimeout = dlclarkFallbackTimeout
		r.reg = reg
	})
	if r.reg != nil {
		return r.reg
	}
	return nil
}

func (r *dlclarkWrapper) CanUse() bool {
	return r != nil && r.getReg() != nil
}

func (r *dlclarkWrapper) Match(b []byte) (bool, error) {
	if r.getReg() == nil {
		return false, errors.New("regexp is nil")
	}
	return r.getReg().MatchString(string(b))
}

func (r *dlclarkWrapper) MatchString(s string) (bool, error) {
	if r.getReg() == nil {
		return false, errors.New("regexp is nil")
	}
	return r.getReg().MatchString(s)
}

func (r *dlclarkWrapper) Find(b []byte) ([]byte, error) {
	res, err := r.FindString(string(b))
	return []byte(res), err
}

func (r *dlclarkWrapper) FindString(s string) (string, error) {
	if r.getReg() == nil {
		return "", errors.New("regexp is nil")
	}
	match, err := r.getReg().FindStringMatch(s)
	if err != nil || match == nil {
		return "", err
	}
	return match.String(), err
}

func (r *dlclarkWrapper) FindAll(b []byte) ([][]byte, error) {
	res, err := r.FindAllString(string(b))
	return lo.Map(res, func(a string, _ int) []byte {
		return []byte(a)
	}), err
}

func (r *dlclarkWrapper) FindAllString(s string) ([]string, error) {
	if r.getReg() == nil {
		return nil, errors.New("regexp is nil")
	}
	matchRes, err := r.getReg().FindStringMatch(s)
	if err != nil {
		return nil, err
	}
	var res []string
	for matchRes != nil {
		res = append(res, matchRes.String())
		matchRes, err = r.getReg().FindNextMatch(matchRes)
		if err != nil {
			return res, err
		}
	}
	return res, nil
}

func (r *dlclarkWrapper) FindSubmatch(b []byte) ([][]byte, error) {
	if r.getReg() == nil {
		return nil, errors.New("regexp is nil")
	}
	res, err := r.FindStringSubmatch(string(b))
	return lo.Map(res, func(a string, _ int) []byte {
		return []byte(a)
	}), err
}

func (r *dlclarkWrapper) FindAllSubmatchIndex(s string) ([][]int, error) {
	if r.getReg() == nil {
		return nil, errors.New("regexp is nil")
	}
	matchRes, err := r.getReg().FindStringMatch(s)
	if err != nil {
		return nil, err
	}
	var results [][]int
	for matchRes != nil {
		for _, g := range matchRes.Groups() {
			results = append(results, []int{g.Index, g.Index + g.Length})
		}
		matchRes, err = r.getReg().FindNextMatch(matchRes)
		if err != nil {
			return results, err
		}
	}
	return results, nil
}

func (r *dlclarkWrapper) FindStringSubmatch(s string) ([]string, error) {
	if r.getReg() == nil {
		return nil, errors.New("regexp is nil")
	}
	matchRes, err := r.getReg().FindStringMatch(s)
	if err != nil {
		return []string{""}, err
	}
	if matchRes == nil {
		return nil, nil
	}
	result := make([]string, matchRes.GroupCount())
	for index, g := range matchRes.Groups() {
		result[index] = g.String()
	}
	return result, nil
}

func (r *dlclarkWrapper) FindAllStringSubmatch(s string) ([][]string, error) {
	if r.getReg() == nil {
		return nil, errors.New("regexp is nil")
	}
	matchRes, err := r.getReg().FindStringMatch(s)
	if err != nil {
		return nil, err
	}
	var results [][]string
	for matchRes != nil {
		groups := matchRes.Groups()
		row := make([]string, len(groups))
		for i, g := range groups {
			row[i] = g.String()
		}
		results = append(results, row)
		matchRes, err = r.getReg().FindNextMatch(matchRes)
		if err != nil {
			return results, err
		}
	}
	return results, nil
}

func (r *dlclarkWrapper) ReplaceAll(src, repl []byte) ([]byte, error) {
	if r.getReg() == nil {
		return nil, errors.New("regexp is nil")
	}
	res, err := r.ReplaceAllString(string(src), string(repl))
	return []byte(res), err
}

func (r *dlclarkWrapper) ReplaceAllString(src, repl string) (string, error) {
	if r.getReg() == nil {
		return "", errors.New("regexp is nil")
	}
	return r.getReg().Replace(src, repl, -1, -1)
}

func (r *dlclarkWrapper) ReplaceAllFunc(src []byte, repl func([]byte) []byte) ([]byte, error) {
	if r.getReg() == nil {
		return nil, errors.New("regexp is nil")
	}
	m, err := r.ReplaceAllStringFunc(string(src), func(s string) string { return string(repl([]byte(s))) })
	return []byte(m), err
}

func (r *dlclarkWrapper) ReplaceAllStringFunc(src string, repl func(string) string) (string, error) {
	if r.getReg() == nil {
		return "", errors.New("regexp is nil")
	}
	return r.getReg().ReplaceFunc(src, func(match dlclark.Match) string {
		return repl(match.String())
	}, 0, -1)
}

func (r *dlclarkWrapper) String() string {
	return r.regexpRaw
}
