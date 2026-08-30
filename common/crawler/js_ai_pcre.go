package crawler

import (
	"fmt"
	"strings"

	pcre2 "github.com/VillanCh/go-pcre2-lite"
	"github.com/yaklang/yaklang/common/minirehs"
)

// aiJSPatternSpec describes one precise PCRE rule and the mandatory literals
// that can cheaply prove the rule worth running. Gates are a safe superset:
// minirehs may admit a false positive, while the PCRE result remains
// authoritative.
type aiJSPatternSpec struct {
	name  string
	expr  string
	gates []string
}

type aiJSPattern struct {
	name  string
	re    *pcre2.Regexp
	gates []string
}

const aiJSPCREEngineErrorSignal = "__pcre_engine_error__"

// aiJSPatternSet is the crawler's two-stage matcher for JavaScript assets:
//
//  1. minirehs performs one case-insensitive existence scan over mandatory
//     literals, avoiding one full-file pass per regular expression.
//  2. Go PCRE Lite performs the precise match only for admitted rules. Its
//     low-level API reports UTF-8 byte offsets, which map directly to source
//     evidence windows.
//
// Both compiled backends are immutable and concurrency-safe. Sets live for the
// process lifetime, matching the package-level rule tables they replace.
type aiJSPatternSet struct {
	patterns           []*aiJSPattern
	gate               *minirehs.Group
	gatePatternIndexes []int
	alwaysOn           []int
}

func mustAIJSPatternSet(specs []aiJSPatternSpec) *aiJSPatternSet {
	set := &aiJSPatternSet{patterns: make([]*aiJSPattern, len(specs))}
	var gateExpressions []string
	for index, spec := range specs {
		if spec.name == "" || spec.expr == "" {
			panic("crawler: empty AI JS PCRE rule")
		}
		re, err := pcre2.Compile(spec.expr, pcre2.CompileOptions{
			// JavaScript assets are scanned as bytes. Disabling UTF validation
			// keeps malformed/minified responses analyzable and preserves byte
			// offsets without changing the ASCII-oriented rules below.
			Caseless:   true,
			MatchLimit: 250_000,
			DepthLimit: 4_096,
		})
		if err != nil {
			panic(fmt.Sprintf("crawler: compile AI JS PCRE rule %q: %v", spec.name, err))
		}
		set.patterns[index] = &aiJSPattern{name: spec.name, re: re, gates: append([]string(nil), spec.gates...)}
		gateCountBefore := len(gateExpressions)
		for _, literal := range spec.gates {
			if literal == "" {
				continue
			}
			gateExpressions = append(gateExpressions, quoteMinirehsLiteral(literal))
			set.gatePatternIndexes = append(set.gatePatternIndexes, index)
		}
		if len(gateExpressions) == gateCountBefore {
			set.alwaysOn = append(set.alwaysOn, index)
		}
	}
	if len(gateExpressions) == 0 {
		return set
	}
	gate, err := minirehs.BuildGroup(
		gateExpressions,
		minirehs.WithGroupBackend("mvs"),
		minirehs.WithGroupExistenceOnly(true),
		minirehs.WithGroupCaseInsensitive(true),
		minirehs.WithGroupMinLiteralLen(2),
	)
	if err != nil {
		for _, pattern := range set.patterns {
			if pattern != nil && pattern.re != nil {
				_ = pattern.re.Close()
			}
		}
		panic(fmt.Sprintf("crawler: compile AI JS minirehs gates: %v", err))
	}
	set.gate = gate
	return set
}

func quoteMinirehsLiteral(literal string) string {
	var builder strings.Builder
	builder.Grow(len(literal) + 4)
	for _, value := range literal {
		if strings.ContainsRune(`\.+*?()|[]{}^$`, value) {
			builder.WriteByte('\\')
		}
		builder.WriteRune(value)
	}
	return builder.String()
}

func (s *aiJSPatternSet) activePatterns(data []byte) []*aiJSPattern {
	if s == nil || len(s.patterns) == 0 || len(data) == 0 {
		return nil
	}
	active := make([]bool, len(s.patterns))
	for _, index := range s.alwaysOn {
		if index >= 0 && index < len(active) {
			active[index] = true
		}
	}
	if s.gate != nil {
		for _, gateIndex := range s.gate.MatchedIndexes(data) {
			if gateIndex < 0 || gateIndex >= len(s.gatePatternIndexes) {
				continue
			}
			patternIndex := s.gatePatternIndexes[gateIndex]
			if patternIndex >= 0 && patternIndex < len(active) {
				active[patternIndex] = true
			}
		}
	}
	result := make([]*aiJSPattern, 0, len(s.patterns))
	for index, pattern := range s.patterns {
		if active[index] {
			result = append(result, pattern)
		}
	}
	return result
}

func (s *aiJSPatternSet) matchedNames(data []byte) map[string]bool {
	matched := make(map[string]bool)
	for _, pattern := range s.activePatterns(data) {
		ok, err := pattern.re.Match(data)
		if err != nil {
			// A matcher resource-limit error is not evidence that the source is
			// clean. Mark it explicitly so the adaptive trigger can fail open and
			// send bounded fallback evidence instead of silently losing an asset.
			matched[aiJSPCREEngineErrorSignal] = true
			continue
		}
		if ok {
			matched[pattern.name] = true
		}
	}
	return matched
}

func (s *aiJSPatternSet) pattern(name string) *aiJSPattern {
	if s == nil {
		return nil
	}
	for _, pattern := range s.patterns {
		if pattern != nil && pattern.name == name {
			return pattern
		}
	}
	return nil
}

func (p *aiJSPattern) findAllIndexes(data []byte, limit int) [][2]int {
	result, _ := p.findAllIndexesWithError(data, limit)
	return result
}

func (p *aiJSPattern) findAllIndexesWithError(data []byte, limit int) ([][2]int, error) {
	if p == nil || p.re == nil || len(data) == 0 || limit == 0 {
		return nil, nil
	}
	matches, err := p.re.FindAll(data, limit)
	if err != nil {
		return nil, err
	}
	result := make([][2]int, 0, len(matches))
	for _, match := range matches {
		if len(match.Groups) == 0 || match.Groups[0].IsUnset() {
			continue
		}
		span := match.Groups[0]
		if span.Start < 0 || span.End < span.Start || span.End > len(data) {
			continue
		}
		result = append(result, [2]int{span.Start, span.End})
	}
	return result, nil
}

func (p *aiJSPattern) captureStrings(data []byte, group, limit int) []string {
	if p == nil || p.re == nil || len(data) == 0 || group < 0 || limit == 0 {
		return nil
	}
	matches, err := p.re.FindAll(data, limit)
	if err != nil {
		return nil
	}
	result := make([]string, 0, len(matches))
	for _, match := range matches {
		if group >= len(match.Groups) || match.Groups[group].IsUnset() {
			continue
		}
		span := match.Groups[group]
		if span.Start < 0 || span.End < span.Start || span.End > len(data) {
			continue
		}
		result = append(result, string(data[span.Start:span.End]))
	}
	return result
}
