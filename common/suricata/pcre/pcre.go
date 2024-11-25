package pcre

import (
	"fmt"
	"github.com/dlclark/regexp2"
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/suricata/data"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
	"github.com/yaklang/yaklang/common/utils/regen"
	"regexp/syntax"
	"strings"
)

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
				log.Warnf("pcre modifier B not implemented, %s may not works as expected\n", pattern)
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
	return &pcre, nil
}

func (p *PCRE) Matcher() (*Matcher, error) {
	matcher, err := regexp2.Compile(p.expr, p.opts)
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
	p.startsWith = beginText
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
	strs := p.generator.Generate()
	if len(strs) == 0 {
		return nil
	}
	return []byte(strs[0])
}
