package regexp_utils

import (
	"github.com/dlclark/regexp2"
	"github.com/yaklang/yaklang/common/utils"
)

type regexpMode string

var (
	RegexpMode1 regexpMode = "re1"
	RegexpMode2 regexpMode = "re2"
)

type YakRegexpUtils struct {
	reg  *RegexpWrapper
	reg2 *Regexp2Wrapper

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
		regexpRaw:    raw,
		priorityMode: RegexpMode1,
	}
	for _, option := range options {
		option(reg)
	}

	reg.reg = NewRegexpWrapper(raw)
	reg.reg2 = NewRegexp2Wrapper(raw, reg.regexpOption)

	return reg
}

func (m *YakRegexpUtils) SetPriority(mode regexpMode) {
	m.priorityMode = mode
}

func (m *YakRegexpUtils) getPriorityRegexp() RegWrapperInterface {
	if m.priorityMode == RegexpMode1 {
		return m.reg
	} else {
		return m.reg2
	}
}

func (m *YakRegexpUtils) getSecondaryRegexp() RegWrapperInterface {
	if m.priorityMode == RegexpMode1 {
		return m.reg2
	} else {
		return m.reg
	}
}

func (m *YakRegexpUtils) String() string {
	return m.regexpRaw
}

func (m *YakRegexpUtils) Match(b []byte) (bool, error) {
	if reg := m.getPriorityRegexp(); reg.CanUse() {
		return reg.Match(b)
	} else if reg := m.getSecondaryRegexp(); reg.CanUse() {
		return reg.Match(b)
	} else {
		return false, utils.Error("yak regexp match fail: no usable regexp")
	}
}

func (m *YakRegexpUtils) MatchString(s string) (bool, error) {
	if reg := m.getPriorityRegexp(); reg.CanUse() {
		return reg.MatchString(s)
	} else if reg := m.getSecondaryRegexp(); reg.CanUse() {
		return reg.MatchString(s)
	} else {
		return false, utils.Error("yak regexp match fail: no usable regexp")
	}
}

func (m *YakRegexpUtils) Find(b []byte) ([]byte, error) {
	if reg := m.getPriorityRegexp(); reg.CanUse() {
		return reg.Find(b)
	} else if reg := m.getSecondaryRegexp(); reg.CanUse() {
		return reg.Find(b)
	} else {
		return nil, utils.Error("yak regexp find fail: no usable regexp")
	}
}

func (m *YakRegexpUtils) FindString(s string) (string, error) {
	if reg := m.getPriorityRegexp(); reg.CanUse() {
		return reg.FindString(s)
	} else if reg := m.getSecondaryRegexp(); reg.CanUse() {
		return reg.FindString(s)
	} else {
		return "", utils.Error("yak regexp find fail: no usable regexp")
	}
}

func (m *YakRegexpUtils) FindSubmatch(b []byte) ([][]byte, error) {
	if reg := m.getPriorityRegexp(); reg.CanUse() {
		return reg.FindSubmatch(b)
	} else if reg := m.getSecondaryRegexp(); reg.CanUse() {
		return reg.FindSubmatch(b)
	} else {
		return nil, utils.Error("yak regexp find fail: no usable regexp")
	}
}

func (m *YakRegexpUtils) FindStringSubmatch(s string) ([]string, error) {
	if reg := m.getPriorityRegexp(); reg.CanUse() {
		return reg.FindStringSubmatch(s)
	} else if reg := m.getSecondaryRegexp(); reg.CanUse() {
		return reg.FindStringSubmatch(s)
	} else {
		return nil, utils.Error("yak regexp find fail: no usable regexp")
	}
}

func (m *YakRegexpUtils) FindAll(b []byte) ([][]byte, error) {
	if reg := m.getPriorityRegexp(); reg.CanUse() {
		return reg.FindAll(b)
	} else if reg := m.getSecondaryRegexp(); reg.CanUse() {
		return reg.FindAll(b)
	} else {
		return nil, utils.Error("yak regexp find fail: no usable regexp")
	}
}

func (m *YakRegexpUtils) FindAllString(s string) ([]string, error) {
	if reg := m.getPriorityRegexp(); reg.CanUse() {
		return reg.FindAllString(s)
	} else if reg := m.getSecondaryRegexp(); reg.CanUse() {
		return reg.FindAllString(s)
	} else {
		return nil, utils.Error("yak regexp find fail: no usable regexp")
	}
}

func (m *YakRegexpUtils) ReplaceAll(src, repl []byte) ([]byte, error) {
	if reg := m.getPriorityRegexp(); reg.CanUse() {
		return reg.ReplaceAll(src, repl)
	} else if reg := m.getSecondaryRegexp(); reg.CanUse() {
		return reg.ReplaceAll(src, repl)
	} else {
		return nil, utils.Error("yak regexp replace fail: no usable regexp")
	}
}

func (m *YakRegexpUtils) ReplaceAllString(src, repl string) (string, error) {
	if reg := m.getPriorityRegexp(); reg.CanUse() {
		return reg.ReplaceAllString(src, repl)
	} else if reg := m.getSecondaryRegexp(); reg.CanUse() {
		return reg.ReplaceAllString(src, repl)
	} else {
		return "", utils.Error("yak regexp replace fail: no usable regexp")
	}
}
