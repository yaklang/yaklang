package pcre

import (
	"fmt"
	"regexp/syntax"
	"strings"
	"sync/atomic"

	regexp2 "github.com/VillanCh/go-pcre2-lite/regexp2"
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/suricata/data"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
	"github.com/yaklang/yaklang/common/utils/regen"
)

const (
	// DefaultMatchLimit bounds the amount of backtracking work for one match
	// attempt. It is deliberately lower than PCRE2's library default.
	DefaultMatchLimit uint32 = 100_000
	// DefaultDepthLimit bounds the backtracking depth for one match attempt.
	DefaultDepthLimit   uint32 = 10_000
	DefaultSubjectLimit        = 8 << 20
	DefaultResultLimit         = 65_536
)

var (
	defaultMatchLimits atomic.Uint64
	// ErrZeroLengthMatchLoop indicates that the regexp backend did not advance
	// after a zero-length match. This protects callers even if backend iteration
	// behavior regresses.
	ErrZeroLengthMatchLoop = errors.New("pcre zero-length match did not advance")
	ErrSubjectLimit        = errors.New("pcre subject size limit exceeded")
	ErrResultLimit         = errors.New("pcre result count limit exceeded")
)

func init() {
	defaultMatchLimits.Store(packMatchLimits(DefaultMatchLimit, DefaultDepthLimit))
}

// SetDefaultMatchLimits configures the limits applied to subsequently compiled
// matchers. Both limits must be non-zero so matchers cannot accidentally fall
// back to the much larger PCRE2 library defaults.
func SetDefaultMatchLimits(matchLimit, depthLimit uint32) error {
	if matchLimit == 0 || depthLimit == 0 {
		return errors.New("pcre match and depth limits must be non-zero")
	}
	defaultMatchLimits.Store(packMatchLimits(matchLimit, depthLimit))
	return nil
}

// GetDefaultMatchLimits returns the limits applied to new matchers.
func GetDefaultMatchLimits() (matchLimit, depthLimit uint32) {
	limits := defaultMatchLimits.Load()
	return uint32(limits >> 32), uint32(limits)
}

func packMatchLimits(matchLimit, depthLimit uint32) uint64 {
	return uint64(matchLimit)<<32 | uint64(depthLimit)
}

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
				return nil, fmt.Errorf("pcre modifier G is not implemented")
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
				return nil, fmt.Errorf("pcre modifier B is not implemented")
			case 'O':
				return nil, fmt.Errorf("pcre modifier O is not implemented")
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
	if err != nil {
		return nil, errors.Wrap(err, "invalid pcre pattern")
	}
	matchLimit, depthLimit := GetDefaultMatchLimits()
	if err := matcher.SetMatchLimits(matchLimit, depthLimit); err != nil {
		return nil, errors.Wrap(err, "configure pcre match limits")
	}
	return &Matcher{
		PCRE:    p,
		matcher: matcher,
	}, nil
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

func (p *Matcher) Match(content []byte) ([]data.Matched, error) {
	if len(content) > DefaultSubjectLimit {
		return nil, ErrSubjectLimit
	}
	matches := make([]data.Matched, 0, 16)
	match, err := p.matcher.FindStringMatch(string(content))
	if err != nil {
		return nil, fmt.Errorf("find first pcre match: %w", err)
	}
	for match != nil {
		if len(matches) >= DefaultResultLimit {
			return nil, ErrResultLimit
		}
		matches = append(matches, data.Matched{
			Pos: match.Index,
			Len: match.Length,
		})

		previous := match
		match, err = p.matcher.FindNextMatch(previous)
		if err != nil {
			return nil, fmt.Errorf("find next pcre match: %w", err)
		}
		if previous.Length == 0 && match != nil && match.Index <= previous.Index {
			return nil, fmt.Errorf("%w at rune offset %d", ErrZeroLengthMatchLoop, previous.Index)
		}
	}
	return matches, nil
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
