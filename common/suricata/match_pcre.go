package suricata

import (
	"fmt"
	"github.com/dlclark/regexp2"
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/regen"
	"golang.org/x/exp/slices"
	"regexp/syntax"
	"strings"
	"time"
)

func init() {
	regexp2.DefaultMatchTimeout = time.Millisecond * 100
}

func newPCREMatch(r *ContentRule) matchHandler {
	pcre, err := ParsePCREStr(r.PCRE)
	if err != nil {
		return nil
	}
	matcher, err := pcre.Matcher()
	if err != nil {
		return nil
	}
	return func(c *matchContext) error {
		var indexes []matched
		buffer := c.GetBuffer(pcre.modifier)

		if pcre.ignoreEndNewline {
			if buffer[len(buffer)-1] == '\n' {
				buffer = buffer[:len(buffer)-1]
			}
		}

		indexes = matcher.Match(buffer)
		if !c.Must(len(indexes) > 0) {
			return nil
		}

		if pcre.startsWith {
			if !c.Must(indexes[0].pos == 0) {
				return nil
			}
		}

		var prevMatch []matched
		loadIfMapEz(c.Value, &prevMatch, "prevMatch")

		if pcre.relative {
			indexes = slices.DeleteFunc(indexes, func(m matched) bool {
				for _, pm := range prevMatch {
					if m.pos == pm.pos+pm.len {
						return false
					}
				}
				return true
			})
			if !c.Must(len(indexes) > 0) {
				return nil
			}
		}

		c.Value["prevMatch"] = indexes
		return nil
	}
}

type PCRE struct {
	expr string

	opts     regexp2.RegexOptions
	modifier Modifier

	relative         bool
	ignoreEndNewline bool
	startsWith       bool
}

type PCREMatcher struct {
	*PCRE
	matcher *regexp2.Regexp
}

type PCREGenerator struct {
	*PCRE
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
			case 'I':
				if normailized {
					pcre.modifier = HTTPUri
				} else {
					pcre.modifier = HTTPUriRaw
				}
			case 'P':
				pcre.modifier = HTTPRequestBody
			case 'Q':
				pcre.modifier = HTTPResponseBody
			case 'H':
				if unnormalized {
					pcre.modifier = HTTPHeaderRaw
				} else {
					pcre.modifier = HTTPHeader
				}
			case 'D':
				// skip
			case 'M':
				pcre.modifier = HTTPMethod
			case 'C':
				pcre.modifier = HTTPCookie
			case 'S':
				pcre.modifier = HTTPStatCode
			case 'Y':
				pcre.modifier = HTTPStatMsg
			case 'B':
				log.Warnf("pcre modifier B not implemented, %s may not works as expected\n", pattern)
			case 'O':
				log.Warnf("pcre modifier O not implemented, %s may not works as expected\n", pattern)
			case 'V':
				pcre.modifier = HTTPUserAgent
			case 'W':
				if unnormalized {
					pcre.modifier = HTTPHostRaw
				} else {
					pcre.modifier = HTTPHost
				}
			default:
				return nil, fmt.Errorf("invalid pcre opt: %s", optstr)
			}
		}
	}
	pcre.opts = opt
	return &pcre, nil
}

func (p *PCRE) Matcher() (*PCREMatcher, error) {
	matcher, err := regexp2.Compile(p.expr, p.opts)
	if err != nil {
		return nil, errors.Wrap(err, "invalid pcre pattern")
	}
	return &PCREMatcher{
		PCRE:    p,
		matcher: matcher,
	}, nil
}

func (p *PCREMatcher) Match(content []byte) []matched {
	var matches []matched
	match, _ := p.matcher.FindStringMatch(string(content))
	for match != nil {
		matches = append(matches, matched{
			pos: match.Index,
			len: match.Length,
		})
		match, _ = p.matcher.FindNextMatch(match)
	}
	return matches
}

func (p *PCRE) Generator() (*PCREGenerator, error) {
	generator, err := regen.NewGeneratorOne(p.expr, &regen.GeneratorArgs{
		Flags: syntax.Perl,
	})
	if err != nil {
		return nil, errors.Wrap(err, "invalid pcre pattern")
	}
	return &PCREGenerator{
		PCRE:      p,
		generator: generator,
	}, nil
}

func (p *PCREGenerator) Generate() []byte {
	strs := p.generator.Generate()
	if len(strs) == 0 {
		return nil
	}
	return []byte(strs[0])
}
