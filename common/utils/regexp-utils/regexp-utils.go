package regexp_utils

import (
	"github.com/dlclark/regexp2"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
	"regexp"
	"sync"
)

type regexpMode string

var (
	RegexpMode1 regexpMode = "re1"
	RegexpMode2 regexpMode = "re2"
)

type YakRegexpUtils struct {
	regexp1     *regexp.Regexp
	regexp1Once *sync.Once

	regexp2     *regexp2.Regexp
	regexp2Once *sync.Once

	regexpRaw    string
	regexpOption regexp2.RegexOptions
	priorityMode regexpMode // re1 or re2
}

type YakRegexpUtilsOption func(*YakRegexpUtils)

func WithPriorityMode(mode regexpMode) YakRegexpUtilsOption {
	return func(m *YakRegexpUtils) {
		m.priorityMode = mode
	}
}

func WithRegexpOption(option regexp2.RegexOptions) YakRegexpUtilsOption {
	return func(m *YakRegexpUtils) {
		m.regexpOption = option
	}
}

func NewYakRegexpUtils(raw string, options ...YakRegexpUtilsOption) *YakRegexpUtils {
	reg := &YakRegexpUtils{
		regexpRaw:   raw,
		regexp1Once: &sync.Once{},
		regexp2Once: &sync.Once{},
	}
	for _, option := range options {
		option(reg)
	}
	return reg
}

func (m *YakRegexpUtils) getRegexp() *regexp.Regexp {
	m.regexp1Once.Do(func() {
		reg, err := regexp.Compile(m.regexpRaw)
		if err != nil {
			return
		}
		m.regexp1 = reg
	})
	if m.regexp1 != nil {
		return m.regexp1
	}
	return nil
}

func (m *YakRegexpUtils) getRegexp2() *regexp2.Regexp {
	m.regexp2Once.Do(func() {
		reg, err := regexp2.Compile(m.regexpRaw, m.regexpOption)
		if err != nil {
			return
		}
		m.regexp2 = reg
	})
	if m.regexp2 != nil {
		return m.regexp2
	}
	return nil
}

func (m *YakRegexpUtils) SetPriority(mode regexpMode) {
	m.priorityMode = mode
}

func (m *YakRegexpUtils) String() string {
	return m.regexpRaw
}

func (m *YakRegexpUtils) Match(b []byte) (bool, error) {
	return m.MatchString(string(b))
}

func (m *YakRegexpUtils) MatchString(s string) (bool, error) {
	if reg := m.getRegexp(); reg != nil {
		return reg.MatchString(s), nil
	} else if reg2 := m.getRegexp2(); reg2 != nil {
		return reg2.MatchString(s)
	} else {
		return false, utils.Error("yak regexp match fail: no usable regexp")
	}
}

func (m *YakRegexpUtils) MatchRune(r []rune) (bool, error) {
	if reg := m.getRegexp(); reg != nil {
		return reg.MatchString(string(r)), nil
	} else if reg2 := m.getRegexp2(); reg2 != nil {
		return reg2.MatchRunes(r)
	} else {
		return false, utils.Error("yak regexp match fail: no usable regexp")
	}
}

func (m *YakRegexpUtils) Find(b []byte) ([]byte, error) {
	res, err := m.FindString(string(b))
	return []byte(res), err
}

func (m *YakRegexpUtils) FindString(s string) (string, error) {
	if reg := m.getRegexp(); reg != nil {
		return reg.FindString(s), nil
	} else if reg2 := m.getRegexp2(); reg2 != nil {
		matchRes, err := reg2.FindStringMatch(s)
		if err != nil {
			return "", err
		}
		return matchRes.String(), nil
	} else {
		return "", utils.Error("yak regexp find fail: no usable regexp")
	}
}

func (m *YakRegexpUtils) FindSubmatch(b []byte) ([][]byte, error) {
	res, err := m.FindStringSubmatch(string(b))
	return lo.Map(res, func(a string, _ int) []byte {
		return []byte(a)
	}), err
}

func (m *YakRegexpUtils) FindStringSubmatch(s string) ([]string, error) {
	if reg := m.getRegexp(); reg != nil {
		return reg.FindStringSubmatch(s), nil
	} else if reg2 := m.getRegexp2(); reg2 != nil {
		matchRes, err := reg2.FindStringMatch(s)
		if err != nil {
			return []string{""}, err
		}
		result := make([]string, matchRes.GroupCount())
		for index, g := range matchRes.Groups() {
			result[index] = g.String()
		}
		return result, nil
	} else {
		return []string{""}, utils.Error("yak regexp find fail: no usable regexp")
	}
}

func (m *YakRegexpUtils) FindAll(b []byte) ([][]byte, error) {
	res, err := m.FindAllString(string(b))

	return lo.Map(res, func(a string, _ int) []byte {
		return []byte(a)
	}), err
}

func (m *YakRegexpUtils) FindAllString(s string) ([]string, error) {
	if reg := m.getRegexp(); reg != nil {
		return reg.FindAllString(s, -1), nil
	} else if reg2 := m.getRegexp2(); reg2 != nil {
		matchRes, err := reg2.FindStringMatch(s)
		if err != nil {
			return nil, err
		}
		var res []string
		for matchRes != nil {
			res = append(res, matchRes.String())
			matchRes, err = m.regexp2.FindNextMatch(matchRes)
			if err != nil {
				return res, err
			}
		}
		return res, nil
	} else {
		return nil, utils.Error("yak regexp findAll fail: no usable regexp")
	}
}

func (m *YakRegexpUtils) FindStringMatch(s string) (*regexp2.Match, error) {
	if reg2 := m.getRegexp2(); reg2 != nil {
		return reg2.FindStringMatch(s)
	} else {
		return nil, utils.Error("yak regexp findAll fail: no usable regexp")
	}
}

func (m *YakRegexpUtils) FindNextMatch(match *regexp2.Match) (*regexp2.Match, error) {
	if reg2 := m.getRegexp2(); reg2 != nil {
		return reg2.FindNextMatch(match)
	} else {
		return nil, utils.Error("yak regexp findAll fail: no usable regexp")
	}
}

func (m *YakRegexpUtils) ReplaceAll(src, repl []byte) ([]byte, error) {
	res, err := m.ReplaceAllString(string(src), string(repl))
	return []byte(res), err
}

func (m *YakRegexpUtils) ReplaceAllString(src, repl string) (string, error) {
	if reg := m.getRegexp(); reg != nil {
		return reg.ReplaceAllString(src, repl), nil
	} else if reg2 := m.getRegexp2(); reg2 != nil {
		return reg2.Replace(src, repl, -1, -1)
	} else {
		return "", utils.Error("yak regexp replace fail: no usable regexp")
	}
}
