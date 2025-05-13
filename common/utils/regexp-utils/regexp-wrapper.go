package regexp_utils

import (
	"errors"
	"regexp"
	"sync"

	"github.com/dlclark/regexp2"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
)

type RegWrapperInterface interface {
	Match(b []byte) (bool, error)
	MatchString(s string) (bool, error)

	Find(b []byte) ([]byte, error)
	FindString(s string) (string, error)

	FindAll(b []byte) ([][]byte, error)
	FindAllString(s string) ([]string, error)

	FindSubmatch(b []byte) ([][]byte, error)
	FindStringSubmatch(s string) ([]string, error)
	FindStringSubmatchIndex(s string) ([][]int, error)

	ReplaceAll(src, repl []byte) ([]byte, error)
	ReplaceAllString(src, repl string) (string, error)

	ReplaceAllFunc(src []byte, repl func([]byte) []byte) ([]byte, error)
	ReplaceAllStringFunc(src string, repl func(string) string) (string, error)

	CanUse() bool
	String() string
}

type RegexpWrapper struct {
	reg       *regexp.Regexp
	regOnce   sync.Once
	regexpRaw string
}

func NewRegexpWrapper(raw string) *RegexpWrapper {
	return &RegexpWrapper{
		regexpRaw: raw,
		regOnce:   sync.Once{},
	}
}

func (r *RegexpWrapper) getReg() *regexp.Regexp {
	r.regOnce.Do(func() {
		reg, err := regexp.Compile(r.regexpRaw)
		if err != nil {
			return
		}
		r.reg = reg
	})
	if r.reg != nil {
		return r.reg
	}
	return nil
}

func (r *RegexpWrapper) CanUse() bool {
	return r != nil && r.getReg() != nil
}

func (r *RegexpWrapper) Match(b []byte) (bool, error) {
	if r.getReg() == nil {
		return false, errors.New("regexp is nil")
	}
	return r.getReg().Match(b), nil
}

func (r *RegexpWrapper) MatchString(s string) (bool, error) {
	if r.getReg() == nil {
		return false, errors.New("regexp is nil")
	}
	return r.getReg().MatchString(s), nil
}

func (r *RegexpWrapper) Find(b []byte) ([]byte, error) {
	if r.getReg() == nil {
		return nil, errors.New("regexp is nil")
	}
	return r.getReg().Find(b), nil
}

func (r *RegexpWrapper) FindString(s string) (string, error) {
	if r.getReg() == nil {
		return "", errors.New("regexp is nil")
	}
	return r.getReg().FindString(s), nil
}

func (r *RegexpWrapper) FindAll(b []byte) ([][]byte, error) {
	if r.getReg() == nil {
		return nil, errors.New("regexp is nil")
	}
	return r.getReg().FindAll(b, -1), nil
}

func (r *RegexpWrapper) FindAllString(s string) ([]string, error) {
	if r.getReg() == nil {
		return nil, errors.New("regexp is nil")
	}
	return r.getReg().FindAllString(s, -1), nil
}

func (r *RegexpWrapper) FindSubmatch(b []byte) ([][]byte, error) {
	if r.getReg() == nil {
		return nil, errors.New("regexp is nil")
	}
	return r.getReg().FindSubmatch(b), nil
}

func (r *RegexpWrapper) FindStringSubmatch(s string) ([]string, error) {
	if r.getReg() == nil {
		return nil, errors.New("regexp is nil")
	}
	return r.getReg().FindStringSubmatch(s), nil
}

func (r *RegexpWrapper) FindStringSubmatchIndex(s string) ([][]int, error) {
	if r.getReg() == nil {
		return nil, errors.New("regexp is nil")
	}
	ret := r.getReg().FindStringSubmatchIndex(s)
	log.Infof("regexp index: %v", ret)
	index := make([][]int, 0, len(ret)/2)
	for i := range len(ret) / 2 {
		if 2*i < len(ret) && ret[2*i] >= 0 {
			index = append(index, []int{ret[2*i], ret[2*i+1]})
		}
	}
	return index, nil
}

func (r *RegexpWrapper) ReplaceAll(src, repl []byte) ([]byte, error) {
	if r.getReg() == nil {
		return nil, errors.New("regexp is nil")
	}
	return r.getReg().ReplaceAll(src, repl), nil
}

func (r *RegexpWrapper) ReplaceAllString(src, repl string) (string, error) {
	if r.getReg() == nil {
		return "", errors.New("regexp is nil")
	}
	return r.getReg().ReplaceAllString(src, repl), nil
}

func (r *RegexpWrapper) ReplaceAllFunc(src []byte, repl func([]byte) []byte) ([]byte, error) {
	if r.getReg() == nil {
		return nil, errors.New("regexp is nil")
	}
	return r.getReg().ReplaceAllFunc(src, repl), nil
}

func (r *RegexpWrapper) ReplaceAllStringFunc(src string, repl func(string) string) (string, error) {
	if r.getReg() == nil {
		return "", errors.New("regexp is nil")
	}
	return r.getReg().ReplaceAllStringFunc(src, repl), nil
}

func (r *RegexpWrapper) String() string {
	return r.regexpRaw
}

type Regexp2Wrapper struct {
	reg       *regexp2.Regexp
	regOnce   sync.Once
	regexpRaw string
	options   regexp2.RegexOptions
}

func NewRegexp2Wrapper(raw string, options regexp2.RegexOptions) *Regexp2Wrapper {
	return &Regexp2Wrapper{
		regexpRaw: raw,
		options:   options,
		regOnce:   sync.Once{},
	}
}

func (r *Regexp2Wrapper) getReg() *regexp2.Regexp {
	r.regOnce.Do(func() {
		reg, err := regexp2.Compile(r.regexpRaw, r.options)
		if err != nil {
			return
		}
		r.reg = reg
	})
	if r.reg != nil {
		return r.reg
	}
	return nil
}

func (r *Regexp2Wrapper) CanUse() bool {
	return r != nil && r.getReg() != nil
}

func (r *Regexp2Wrapper) Match(b []byte) (bool, error) {
	if r.getReg() == nil {
		return false, errors.New("regexp is nil")
	}
	return r.getReg().MatchString(string(b))
}

func (r *Regexp2Wrapper) MatchString(s string) (bool, error) {
	if r.getReg() == nil {
		return false, errors.New("regexp is nil")
	}
	return r.getReg().MatchString(s)
}

func (r *Regexp2Wrapper) Find(b []byte) ([]byte, error) {
	res, err := r.FindString(string(b))
	return []byte(res), err
}

func (r *Regexp2Wrapper) FindString(s string) (string, error) {
	if r.getReg() == nil {
		return "", errors.New("regexp is nil")
	}
	match, err := r.getReg().FindStringMatch(s)
	if err != nil || match == nil {
		return "", err
	}
	return match.String(), err
}

func (r *Regexp2Wrapper) FindAll(b []byte) ([][]byte, error) {
	res, err := r.FindAllString(string(b))

	return lo.Map(res, func(a string, _ int) []byte {
		return []byte(a)
	}), err
}

func (r *Regexp2Wrapper) FindAllString(s string) ([]string, error) {
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

func (r *Regexp2Wrapper) FindSubmatch(b []byte) ([][]byte, error) {
	if r.getReg() == nil {
		return nil, errors.New("regexp is nil")
	}
	res, err := r.FindStringSubmatch(string(b))
	return lo.Map(res, func(a string, _ int) []byte {
		return []byte(a)
	}), err
}

func (r *Regexp2Wrapper) FindStringSubmatchIndex(s string) ([][]int, error) {
	if r.getReg() == nil {
		return nil, errors.New("regexp is nil")
	}
	matchRes, err := r.getReg().FindStringMatch(s)
	if err != nil {
		return nil, err
	}
	var results [][]int
	for _, g := range matchRes.Groups() {
		results = append(results, []int{g.Index, g.Index + g.Length})
	}
	return results, nil
}

func (r *Regexp2Wrapper) FindStringSubmatch(s string) ([]string, error) {
	if r.getReg() == nil {
		return nil, errors.New("regexp is nil")
	}
	matchRes, err := r.getReg().FindStringMatch(s)
	if err != nil {
		return []string{""}, err
	}
	result := make([]string, matchRes.GroupCount())
	for index, g := range matchRes.Groups() {
		result[index] = g.String()
	}
	return result, nil
}

func (r *Regexp2Wrapper) ReplaceAll(src, repl []byte) ([]byte, error) {
	if r.getReg() == nil {
		return nil, errors.New("regexp is nil")
	}
	res, err := r.ReplaceAllString(string(src), string(repl))
	return []byte(res), err
}

func (r *Regexp2Wrapper) ReplaceAllString(src, repl string) (string, error) {
	if r.getReg() == nil {
		return "", errors.New("regexp is nil")
	}
	return r.getReg().Replace(src, repl, -1, -1)
}

func (r *Regexp2Wrapper) ReplaceAllFunc(src []byte, repl func([]byte) []byte) ([]byte, error) {
	if r.getReg() == nil {
		return nil, errors.New("regexp is nil")
	}

	m, err := r.ReplaceAllStringFunc(string(src), func(s string) string { return string(repl([]byte(s))) })
	return []byte(m), err
}

func (r *Regexp2Wrapper) ReplaceAllStringFunc(src string, repl func(string) string) (string, error) {
	if r.getReg() == nil {
		return "", errors.New("regexp is nil")
	}
	return r.getReg().ReplaceFunc(src, func(match regexp2.Match) string {
		return repl(match.String())
	}, 0, -1)
}

func (r *Regexp2Wrapper) String() string {
	return r.regexpRaw
}
