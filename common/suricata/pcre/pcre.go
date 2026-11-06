package pcre

import (
	"fmt"
	"regexp/syntax"
	"strings"
	"sync/atomic"

	pcre2lite "github.com/VillanCh/go-pcre2-lite"
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

	ignoreCase bool
	multiline  bool
	singleline bool
	extended   bool
	modifier   modifier.Modifier

	relative      bool
	dollarEndOnly bool
	startsWith    bool
	rawBytes      bool
	ungreedy      bool
}

type Matcher struct {
	*PCRE
	matcher *pcre2lite.Regexp
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
	pcre.expr = pattern[1:idx]
	// Suricata PCRE delimiters are sometimes escaped as `\/`; LastIndexByte
	// then leaves the escape backslash at the end of the expression. Remove
	// only an odd trailing backslash, preserving an escaped literal backslash.
	if trailing := trailingBackslashCount(pcre.expr); trailing%2 == 1 {
		pcre.expr = pcre.expr[:len(pcre.expr)-1]
	}
	pcre.expr = normalizeLegacyCharacterClassRanges(pcre.expr)
	// The matching backend is 8-bit PCRE2. Go's regexp/syntax rejects
	// PCRE constructs such as lookarounds, so use it only for anchor analysis
	// and let Matcher validate the complete pattern.
	beginText := strings.HasPrefix(pcre.expr, "^") || strings.HasPrefix(pcre.expr, `\A`)
	if re, err := syntax.Parse(pcre.expr, syntax.Perl); err == nil {
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
	}

	if idx != len(pattern)-1 {
		optstr := pattern[idx+1:]
		for _, v := range optstr {
			switch v {
			case 'i':
				pcre.ignoreCase = true
			case 'm':
				pcre.multiline = true
			case 's':
				pcre.singleline = true
			case 'x':
				pcre.extended = true
			case 'A':
				pcre.startsWith = true
			case 'E':
				pcre.dollarEndOnly = true
			case 'G':
				pcre.ungreedy = true
			case 'R':
				pcre.relative = true
			case 'U':
				pcre.modifier = modifier.HTTPUri
			case 'I':
				pcre.modifier = modifier.HTTPUriRaw
			case 'P':
				pcre.modifier = modifier.HTTPRequestBody
			case 'Q':
				pcre.modifier = modifier.HTTPResponseBody
			case 'H':
				pcre.modifier = modifier.HTTPHeader
			case 'D':
				pcre.modifier = modifier.HTTPHeaderRaw
			case 'M':
				pcre.modifier = modifier.HTTPMethod
			case 'C':
				pcre.modifier = modifier.HTTPCookie
			case 'S':
				pcre.modifier = modifier.HTTPStatCode
			case 'Y':
				pcre.modifier = modifier.HTTPStatMsg
			case 'B':
				pcre.rawBytes = true
			case 'O':
				// Suricata uses O to opt into its configured PCRE match
				// limits. Matchers always enforce those limits here.

			case 'V':
				pcre.modifier = modifier.HTTPUserAgent
			case 'W':
				pcre.modifier = modifier.HTTPHost
			default:
				return nil, fmt.Errorf("invalid pcre opt: %s", optstr)
			}
		}
	}
	pcre.startsWith = pcre.startsWith || beginText
	return &pcre, nil
}

func (p *PCRE) RawBytes() bool {
	return p.rawBytes
}

func trailingBackslashCount(value string) int {
	count := 0
	for index := len(value) - 1; index >= 0 && value[index] == '\\'; index-- {
		count++
	}
	return count
}

func (p *PCRE) Matcher() (*Matcher, error) {
	matchLimit, depthLimit := GetDefaultMatchLimits()
	matcher, err := pcre2lite.Compile(p.expr, pcre2lite.CompileOptions{
		Caseless:      p.ignoreCase,
		Multiline:     p.multiline,
		DotAll:        p.singleline,
		Extended:      p.extended,
		Ungreedy:      p.ungreedy,
		DollarEndOnly: p.dollarEndOnly,
		MatchLimit:    matchLimit,
		DepthLimit:    depthLimit,
	})
	if err != nil {
		return nil, errors.Wrap(err, "invalid pcre pattern")
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

// IgnoreEndNewline is retained for compatibility. Suricata's /E option is
// PCRE2_DOLLAR_ENDONLY, exposed through DollarEndOnly.
func (p *PCRE) IgnoreEndNewline() bool {
	return p.dollarEndOnly
}

func (p *PCRE) DollarEndOnly() bool {
	return p.dollarEndOnly
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
	return p.ignoreCase
}

func (p *Matcher) Match(content []byte) ([]data.Matched, error) {
	if len(content) > DefaultSubjectLimit {
		return nil, ErrSubjectLimit
	}
	// Request one extra match so the result budget can be reported exactly.
	found, err := p.matcher.FindAll(content, DefaultResultLimit+1)
	if err != nil {
		return nil, fmt.Errorf("match pcre: %w", err)
	}
	if len(found) > DefaultResultLimit {
		return nil, ErrResultLimit
	}
	matches := make([]data.Matched, 0, len(found))
	for _, match := range found {
		if len(match.Groups) == 0 {
			continue
		}
		span := match.Groups[0]
		if span.IsUnset() {
			continue
		}
		matches = append(matches, data.Matched{
			Pos: span.Start,
			Len: span.End - span.Start,
		})
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
