package pcre

import (
	"fmt"
	"time"

	dlclark "github.com/dlclark/regexp2"
	regexp2 "github.com/VillanCh/go-pcre2-lite/regexp2"
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/suricata/data"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
	"github.com/yaklang/yaklang/common/utils/regen"
	"regexp"
	"regexp/syntax"
	"strings"
)

// dlclarkRe2 是 dlclark/regexp2 的编译产物类型 (指针), 与 go-pcre2-lite/regexp2.Regexp 类型独立.
// 用于 suricata pcre 的终极兜底 (PCRE2 与 Go RE2 都编译失败时).
type dlclarkRe2 = *dlclark.Regexp

// dlclarkFallbackTimeout: dlclark 兜底的单次匹配超时, 防灾难回溯挂死; suricata 规则匹配
// 可能扫大报文, 留宽裕 5 分钟. dlclark 默认 NoTimeout, 不设上限会被 (a+)+ 类 pattern 拖垮.
const dlclarkFallbackTimeout = 5 * time.Minute

type PCRE struct {
	expr string

	opts     regexp2.RegexOptions
	modifier modifier.Modifier

	relative         bool
	ignoreEndNewline bool
	startsWith       bool
}

type Matcher struct {
	*PCRE
	matcher *regexp2.Regexp
	// re2 回退: PCRE2 比 dlclark/.NET 更严格 (如 [\d\w-_] 被判 invalid range), 而 Go RE2 与
	// 生成器侧 (regen 用 RE2) 一致且更宽松. 当 PCRE2 编译失败但 RE2 能编译时改走 re2, 保住真实
	// suricata 规则的匹配能力. lookbehind/backref 等 RE2 不支持者 RE2 同样失败, 此时仍返回原错.
	re2 *regexp.Regexp
	// dlclark 兜底: 真实 suricata 规则集里存在 RE2 也不接受的畸形/边界构造 (如转义损坏的
	// [\d\w-_]{50,} 被规则源误写成 [\d\w-_]50,}). dlclark/.NET 宽容这类, 作为终极兜底
	// 保住规则可加载; 仅在 PCRE2 与 RE2 都编译失败时启用.
	dlclarkMatcher dlclarkRe2
}

type Generator struct {
	*PCRE
	reSyntax  *syntax.Regexp
	generator regen.Generator
}

func ParsePCREStr(pattern string) (*PCRE, error) {
	if len(pattern) < 3 {
		return nil, errors.New("invalid pcre pattern")
	}
	idx := strings.LastIndexByte(pattern, '/')
	if idx == -1 {
		return nil, errors.New("invalid pcre pattern")
	}
	var pcre PCRE
	var opt regexp2.RegexOptions = 0
	normailized := strings.Contains(pattern[idx+1:], "U")
	unnormalized := strings.Contains(pattern[idx+1:], "D")
	pcre.expr = pattern[1:idx]
	re, err := syntax.Parse(pcre.expr, syntax.Perl)
	if err != nil {
		return nil, errors.Wrap(err, "invalid pcre pattern")
	}

	var beginText bool
	var walkPositionControlOp func(regexpIns *syntax.Regexp)
	walkPositionControlOp = func(regexpIns *syntax.Regexp) {
		if regexpIns.Op == syntax.OpConcat {
			if len(regexpIns.Sub) > 0 {
				walkPositionControlOp(regexpIns.Sub[0])
				return
			}
		}
		if regexpIns.Op == syntax.OpBeginText {
			beginText = true
		}
	}
	walkPositionControlOp(re)

	if idx != len(pattern)-1 {
		optstr := pattern[idx+1:]
		for _, v := range optstr {
			switch v {
			case 'i':
				opt |= regexp2.IgnoreCase
			case 'm':
				opt |= regexp2.Multiline
			case 's':
				opt |= regexp2.Singleline
			case 'A':
				pcre.startsWith = true
			case 'E':
				pcre.ignoreEndNewline = true
			case 'G':
				log.Warnf("pcre modifier G not implemented, %s may not works as expected\n", pattern)
				// Inverts the greediness.
				// not implemented
			case 'R':
				pcre.relative = true
			case 'U':
				// skip
				pcre.modifier = modifier.HTTPUri
			case 'I':
				if normailized {
					pcre.modifier = modifier.HTTPUri
				} else {
					pcre.modifier = modifier.HTTPUriRaw
				}
			case 'P':
				pcre.modifier = modifier.HTTPRequestBody
			case 'Q':
				pcre.modifier = modifier.HTTPResponseBody
			case 'H':
				if unnormalized {
					pcre.modifier = modifier.HTTPHeaderRaw
				} else {
					pcre.modifier = modifier.HTTPHeader
				}
			case 'D':
				// skip
			case 'M':
				pcre.modifier = modifier.HTTPMethod
			case 'C':
				pcre.modifier = modifier.HTTPCookie
			case 'S':
				pcre.modifier = modifier.HTTPStatCode
			case 'Y':
				pcre.modifier = modifier.HTTPStatMsg
			case 'B':
				//log.Warnf("pcre modifier B not implemented, %s may not works as expected\n", pattern)
			case 'O':
				log.Warnf("pcre modifier O not implemented, %s may not works as expected\n", pattern)
			case 'V':
				pcre.modifier = modifier.HTTPUserAgent
			case 'W':
				if unnormalized {
					pcre.modifier = modifier.HTTPHostRaw
				} else {
					pcre.modifier = modifier.HTTPHost
				}
			default:
				return nil, fmt.Errorf("invalid pcre opt: %s", optstr)
			}
		}
	}
	pcre.opts = opt
	pcre.startsWith = pcre.startsWith || beginText
	return &pcre, nil
}

func (p *PCRE) Matcher() (*Matcher, error) {
	matcher, err := regexp2.Compile(p.expr, p.opts)
	if err == nil {
		return &Matcher{
			PCRE:    p,
			matcher: matcher,
		}, nil
	}
	// PCRE2 编译失败: 尝试 Go RE2 回退 (更宽松, 与生成器一致). RE2 也无法表达者 (lookbehind/
	// backref) 回退同样失败, 返回原 PCRE2 错误, 保持原有失败语义.
	if re2, re2Err := compileRE2Fallback(p.expr, p.opts); re2Err == nil {
		log.Debugf("pcre2 compile failed (%v), fell back to RE2 for: %s", err, p.expr)
		return &Matcher{
			PCRE: p,
			re2:  re2,
		}, nil
	}
	// 终极兜底: dlclark/.NET 语义. 真实 suricata 规则集里存在 PCRE2 与 RE2 都拒绝的畸形/边界
	// 构造 (典型: 规则源转义损坏导致 [\d\w\-_]{50,} 被写成 [\d\w-_]50,}, RE2 判 invalid range).
	// dlclark 宽容这类, 保住规则可加载与匹配; 用 MatchTimeout 防灾难回溯.
	if dc, dcErr := dlclark.Compile(p.expr, dlclark.RegexOptions(p.opts)); dcErr == nil {
		dc.MatchTimeout = dlclarkFallbackTimeout
		log.Debugf("pcre2 and RE2 both failed (pcre2: %v), fell back to dlclark for: %s", err, p.expr)
		return &Matcher{
			PCRE:           p,
			dlclarkMatcher: dc,
		}, nil
	}
	return nil, errors.Wrap(err, "invalid pcre pattern")
}

// compileRE2Fallback 把 PCRE2 选项翻成 RE2 内联标志后用 Go regexp 编译, 作为 PCRE2 不可编译时的回退.
func compileRE2Fallback(expr string, opts regexp2.RegexOptions) (*regexp.Regexp, error) {
	var flags string
	if opts&regexp2.IgnoreCase != 0 {
		flags += "i"
	}
	if opts&regexp2.Multiline != 0 {
		flags += "m"
	}
	if opts&regexp2.Singleline != 0 {
		flags += "s"
	}
	pattern := expr
	if flags != "" {
		pattern = "(?" + flags + ")" + expr
	}
	return regexp.Compile(pattern)
}

func (p *PCRE) Modifier() modifier.Modifier {
	return p.modifier
}

func (p *PCRE) Relative() bool {
	return p.relative
}

func (p *PCRE) IgnoreEndNewline() bool {
	return p.ignoreEndNewline
}

func (p *PCRE) StartsWith() bool {
	return p.startsWith
}

func (p *PCRE) Generator() (*Generator, error) {
	re, err := syntax.Parse(p.expr, syntax.Perl)
	if err != nil {
		return nil, err
	}
	generator, err := regen.NewGeneratorOne(p.expr, &regen.GeneratorArgs{
		Flags: syntax.Perl,
	})
	if err != nil {
		return nil, errors.Wrap(err, "invalid pcre pattern")
	}
	return &Generator{
		reSyntax:  re,
		PCRE:      p,
		generator: generator,
	}, nil
}

func (p *PCRE) IgnoreCase() bool {
	return p.opts&regexp2.IgnoreCase != 0
}

func (p *Matcher) Match(content []byte) []data.Matched {
	var matches []data.Matched
	if p.matcher == nil {
		// dlclark 兜底路径 (PCRE2 与 RE2 都编译失败时启用).
		if p.dlclarkMatcher != nil {
			match, _ := p.dlclarkMatcher.FindStringMatch(string(content))
			for match != nil {
				matches = append(matches, data.Matched{
					Pos: match.Index,
					Len: match.Length,
				})
				match, _ = p.dlclarkMatcher.FindNextMatch(match)
			}
			return matches
		}
		// RE2 回退路径 (PCRE2 编译失败时启用).
		for _, loc := range p.re2.FindAllIndex(content, -1) {
			matches = append(matches, data.Matched{
				Pos: loc[0],
				Len: loc[1] - loc[0],
			})
		}
		return matches
	}
	match, _ := p.matcher.FindStringMatch(string(content))
	for match != nil {
		matches = append(matches, data.Matched{
			Pos: match.Index,
			Len: match.Length,
		})
		match, _ = p.matcher.FindNextMatch(match)
	}
	return matches
}

func (p *Generator) Generate() []byte {
	if p.generator == nil {
		return nil
	}
	strs := p.generator.Generate()
	if len(strs) == 0 {
		return nil
	}
	return []byte(strs[0])
}
