package regexp_utils

import (
	"fmt"
	"time"

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

func RegexpAppendOption(raw string, option regexp2.RegexOptions) string {
	modeString := ""
	if option&regexp2.IgnoreCase != 0 {
		modeString = modeString + "i"
	}
	if option&regexp2.Singleline != 0 {
		modeString = modeString + "s"
	}
	if option&regexp2.Multiline != 0 {
		modeString = modeString + "m"
	}
	if modeString != "" {
		return "(?" + modeString + ")" + raw
	}
	return raw
}

func NewYakRegexpUtils(raw string, options ...YakRegexpUtilsOption) *YakRegexpUtils {
	reg := &YakRegexpUtils{
		regexpRaw:    raw,
		priorityMode: RegexpMode1,
		regexpOption: regexp2.None,
	}
	for _, option := range options {
		option(reg)
	}

	reg.reg = NewRegexpWrapper(RegexpAppendOption(raw, reg.regexpOption))
	reg.reg2 = NewRegexp2Wrapper(raw, reg.regexpOption)

	return reg
}

func (m *YakRegexpUtils) CanUse() bool {
	if reg := m.getPriorityRegexp(); reg.CanUse() {
		return true
	}
	if reg := m.getSecondaryRegexp(); reg.CanUse() {
		return true
	}
	return false
}

func (m *YakRegexpUtils) Hash() string {
	return utils.CalcMd5(fmt.Sprintf("%s|%s|%d", m.regexpRaw, m.priorityMode, m.regexpOption))
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

func (m *YakRegexpUtils) FindAllSubmatchIndex(s string) ([][]int, error) {
	if reg := m.getPriorityRegexp(); reg.CanUse() {
		return reg.FindAllSubmatchIndex(s)
	} else if reg := m.getSecondaryRegexp(); reg.CanUse() {
		return reg.FindAllSubmatchIndex(s)
	} else {
		return nil, utils.Errorf("yak regexp find fail: no usable regexp: %s", s)
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

func (m *YakRegexpUtils) ReplaceAllFunc(src []byte, repl func([]byte) []byte) ([]byte, error) {
	if reg := m.getPriorityRegexp(); reg.CanUse() {
		return reg.ReplaceAllFunc(src, repl)
	} else if reg := m.getSecondaryRegexp(); reg.CanUse() {
		return reg.ReplaceAllFunc(src, repl)
	} else {
		return nil, utils.Error("yak regexp replace fail: no usable regexp")
	}
}

func (m *YakRegexpUtils) ReplaceAllStringFunc(src string, repl func(string) string) (string, error) {
	if reg := m.getPriorityRegexp(); reg.CanUse() {
		return reg.ReplaceAllStringFunc(src, repl)
	} else if reg := m.getSecondaryRegexp(); reg.CanUse() {
		return reg.ReplaceAllStringFunc(src, repl)
	} else {
		return "", utils.Error("yak regexp replace fail: no usable regexp")
	}
}

var DefaultYakRegexpManager = NewYakRegexpManager()

type YakRegexpManager struct {
	regs *utils.Cache[*YakRegexpUtils]
}

func NewYakRegexpManager() *YakRegexpManager {
	return &YakRegexpManager{
		regs: utils.NewTTLCache[*YakRegexpUtils](5 * time.Minute),
	}
}

func (manager *YakRegexpManager) GetYakRegexp(raw string, options ...YakRegexpUtilsOption) *YakRegexpUtils {
	reg := NewYakRegexpUtils(raw, options...)
	if reg, ok := manager.regs.Get(reg.Hash()); ok {
		return reg
	}
	manager.regs.Set(raw, reg)
	return reg
}
