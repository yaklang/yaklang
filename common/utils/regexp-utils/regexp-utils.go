package regexp_utils

import (
	"fmt"
	"regexp"
	"time"

	"github.com/dlclark/regexp2"
	"github.com/samber/lo"
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
	return m.getUsableRegexp() != nil
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

// getUsableRegexp 返回第一个可用的 regexp（优先标准库，失败时回退 regexp2 支持 lookbehind/lookahead）
func (m *YakRegexpUtils) getUsableRegexp() RegWrapperInterface {
	if reg := m.getPriorityRegexp(); reg.CanUse() {
		return reg
	}
	if reg := m.getSecondaryRegexp(); reg.CanUse() {
		return reg
	}
	return nil
}

func (m *YakRegexpUtils) String() string {
	return m.regexpRaw
}

func (m *YakRegexpUtils) Match(b []byte) (bool, error) {
	if reg := m.getUsableRegexp(); reg != nil {
		return reg.Match(b)
	}
	return false, utils.Error("yak regexp match fail: no usable regexp")
}

func (m *YakRegexpUtils) MatchString(s string) (bool, error) {
	if reg := m.getUsableRegexp(); reg != nil {
		return reg.MatchString(s)
	}
	return false, utils.Error("yak regexp match fail: no usable regexp")
}

func (m *YakRegexpUtils) Find(b []byte) ([]byte, error) {
	if reg := m.getUsableRegexp(); reg != nil {
		return reg.Find(b)
	}
	return nil, utils.Error("yak regexp find fail: no usable regexp")
}

func (m *YakRegexpUtils) FindString(s string) (string, error) {
	if reg := m.getUsableRegexp(); reg != nil {
		return reg.FindString(s)
	}
	return "", utils.Error("yak regexp find fail: no usable regexp")
}

func (m *YakRegexpUtils) FindSubmatch(b []byte) ([][]byte, error) {
	if reg := m.getUsableRegexp(); reg != nil {
		return reg.FindSubmatch(b)
	}
	return nil, utils.Error("yak regexp find fail: no usable regexp")
}

func (m *YakRegexpUtils) FindStringSubmatch(s string) ([]string, error) {
	if reg := m.getUsableRegexp(); reg != nil {
		return reg.FindStringSubmatch(s)
	}
	return nil, utils.Error("yak regexp find fail: no usable regexp")
}

func (m *YakRegexpUtils) FindAllStringSubmatch(s string) ([][]string, error) {
	if reg := m.getUsableRegexp(); reg != nil {
		return reg.FindAllStringSubmatch(s)
	}
	return nil, utils.Errorf("yak regexp find fail: no usable regexp: %s", m.regexpRaw)
}

func (m *YakRegexpUtils) FindAllSubmatchIndex(s string) ([][]int, error) {
	if reg := m.getUsableRegexp(); reg != nil {
		return reg.FindAllSubmatchIndex(s)
	}
	return nil, utils.Errorf("yak regexp find fail: no usable regexp: %s", m.regexpRaw)
}

func (m *YakRegexpUtils) FindAll(b []byte) ([][]byte, error) {
	if reg := m.getUsableRegexp(); reg != nil {
		return reg.FindAll(b)
	}
	return nil, utils.Error("yak regexp find fail: no usable regexp")
}

func (m *YakRegexpUtils) FindAllString(s string) ([]string, error) {
	if reg := m.getUsableRegexp(); reg != nil {
		return reg.FindAllString(s)
	}
	return nil, utils.Error("yak regexp find fail: no usable regexp")
}

func (m *YakRegexpUtils) ReplaceAll(src, repl []byte) ([]byte, error) {
	if reg := m.getUsableRegexp(); reg != nil {
		return reg.ReplaceAll(src, repl)
	}
	return nil, utils.Error("yak regexp replace fail: no usable regexp")
}

func (m *YakRegexpUtils) ReplaceAllString(src, repl string) (string, error) {
	if reg := m.getUsableRegexp(); reg != nil {
		return reg.ReplaceAllString(src, repl)
	}
	return "", utils.Error("yak regexp replace fail: no usable regexp")
}

func (m *YakRegexpUtils) ReplaceAllFunc(src []byte, repl func([]byte) []byte) ([]byte, error) {
	if reg := m.getUsableRegexp(); reg != nil {
		return reg.ReplaceAllFunc(src, repl)
	}
	return nil, utils.Error("yak regexp replace fail: no usable regexp")
}

func (m *YakRegexpUtils) ReplaceAllStringFunc(src string, repl func(string) string) (string, error) {
	if reg := m.getUsableRegexp(); reg != nil {
		return reg.ReplaceAllStringFunc(src, repl)
	}
	return "", utils.Error("yak regexp replace fail: no usable regexp")
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

// ResolveGroupIndices 解析 groupNames 为数字索引。仅标准 regexp 支持 SubexpIndex；
// 当 pattern 含 lookbehind 等 regexp 不支持语法时，regexp.Compile 失败，仅返回 groupIndices。
func ResolveGroupIndices(pattern string, groupIndices []int, groupNames []string) []int {
	resolved := append([]int{}, groupIndices...)
	if re, err := regexp.Compile(pattern); err == nil {
		for _, name := range groupNames {
			if n := re.SubexpIndex(name); n >= 0 {
				resolved = append(resolved, n)
			}
		}
	}
	return lo.Uniq(resolved)
}
