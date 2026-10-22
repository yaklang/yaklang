package sfpattern

import (
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/gobwas/glob"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/minirehs"
)

// pathWhere says which string a converted path regexp is allowed to match.
// compilePathMatcher ORs several views of one path; each converted expression
// keeps only the views it actually replaces.
type pathWhere uint8

const (
	pathFull pathWhere = 1 << iota
	pathBase
	pathSlash
	pathSlashBase
)

type pathRegex struct {
	expr  string
	where pathWhere
}

type pathGroup struct {
	expr     string
	matchAll bool
	match    func(string) bool
	patterns []int
}

// pathIndex answers "which content patterns apply to this file" with one
// multi-regexp scan when a path expression converts without changing
// compilePathMatcher. Expressions that are both a glob and a regexp, or whose
// glob is not a simple *, ?, literal pattern, keep the existing matcher.
type pathIndex struct {
	groups  []pathGroup
	db      minirehs.Database
	where   []pathWhere
	groupOf []int
}

func buildPathIndex(searches map[string]sourceSearch) *pathIndex {
	idx := &pathIndex{}
	if len(searches) == 0 {
		return idx
	}
	keys := make([]string, 0, len(searches))
	for key := range searches {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	order := make([]string, 0, len(searches))
	byExpr := map[string]*pathGroup{}
	for _, key := range keys {
		search := searches[key]
		group := byExpr[search.path]
		if group == nil {
			group = &pathGroup{expr: search.path}
			byExpr[search.path] = group
			order = append(order, search.path)
		}
		group.patterns = append(group.patterns, search.positive...)
		group.patterns = append(group.patterns, search.negative...)
	}

	var compiled []minirehs.Pattern
	for _, expr := range order {
		group := byExpr[expr]
		regexes, matchAll, ok := pathRegexes(expr)
		switch {
		case matchAll:
			group.matchAll = true
		case ok:
			groupIndex := len(idx.groups)
			for _, re := range regexes {
				id := len(compiled)
				compiled = append(compiled, minirehs.Pattern{ID: minirehs.PatternID(id), Expr: re.expr})
				idx.where = append(idx.where, re.where)
				idx.groupOf = append(idx.groupOf, groupIndex)
			}
		default:
			group.match = compilePathMatcher(expr)
		}
		idx.groups = append(idx.groups, *group)
	}
	if len(compiled) == 0 {
		return idx
	}
	db, err := minirehs.Compile(compiled, minirehs.WithBackend(minirehs.BackendEngine))
	if err != nil {
		log.Debugf("source path index: keep per-path matchers: %v", err)
		for i := range idx.groups {
			if idx.groups[i].matchAll || idx.groups[i].match != nil {
				continue
			}
			idx.groups[i].match = compilePathMatcher(idx.groups[i].expr)
		}
		return &pathIndex{groups: idx.groups}
	}
	idx.db = db
	return idx
}

func (idx *pathIndex) Close() error {
	if idx != nil && idx.db != nil {
		return idx.db.Close()
	}
	return nil
}

// newPathState prepares the caller's reusable path scratch. It is nil when
// every path expression is match-all or a fallback matcher.
func (idx *pathIndex) newPathState() (*pathScanState, error) {
	if idx == nil || idx.db == nil {
		return nil, nil
	}
	scratch, err := idx.db.NewScratch()
	if err != nil {
		return nil, err
	}
	return &pathScanState{scratch: scratch, matched: make([]bool, len(idx.groups))}, nil
}

type pathScanState struct {
	scratch minirehs.Scratch
	matched []bool
	probes  []pathProbe
}

type pathProbe struct {
	text  string
	where pathWhere
}

func (s *pathScanState) Close() error {
	if s != nil && s.scratch != nil {
		return s.scratch.Close()
	}
	return nil
}

// allow reports content-pattern ids whose positive or negative expression can
// see this file. scratch is owned by the caller and reused across files; a nil
// scratch means the regex index is unavailable and only match-all / fallback
// groups apply.
func (idx *pathIndex) allow(file string, scratch minirehs.Scratch, n int) []bool {
	allowed := make([]bool, n)
	if idx == nil {
		return allowed
	}
	var state *pathScanState
	if scratch != nil {
		state = &pathScanState{scratch: scratch, matched: make([]bool, len(idx.groups))}
	}
	idx.fillAllow(file, state, allowed)
	return allowed
}

// fillAllow reuses allowed and state.matched across files. The caller clears
// allowed before the next file; this method clears its own group buffer.
func (idx *pathIndex) fillAllow(file string, state *pathScanState, allowed []bool) {
	if idx == nil {
		return
	}
	var matched []bool
	if state != nil {
		clear(state.matched)
		matched = state.matched
	} else {
		matched = make([]bool, len(idx.groups))
	}
	for i, group := range idx.groups {
		if group.matchAll {
			matched[i] = true
		}
	}
	if idx.db != nil && state != nil && state.scratch != nil {
		slash := filepath.ToSlash(file)
		state.probes = append(state.probes[:0], pathProbe{file, pathFull})
		add := func(text string, where pathWhere) {
			for i := range state.probes {
				if state.probes[i].text == text {
					state.probes[i].where |= where
					return
				}
			}
			state.probes = append(state.probes, pathProbe{text, where})
		}
		add(path.Base(file), pathBase)
		add(slash, pathSlash)
		add(path.Base(slash), pathSlashBase)
		for _, probe := range state.probes {
			_ = idx.db.Scan([]byte(probe.text), state.scratch, func(m minirehs.Match) bool {
				id := int(m.ID)
				if id < 0 || id >= len(idx.where) || idx.where[id]&probe.where == 0 {
					return true
				}
				matched[idx.groupOf[id]] = true
				return true
			})
		}
	}
	for i, group := range idx.groups {
		if group.match != nil && group.match(file) {
			matched[i] = true
		}
		if !matched[i] {
			continue
		}
		for _, id := range group.patterns {
			if id >= 0 && id < len(allowed) {
				allowed[id] = true
			}
		}
	}
}

// pathRegexes converts a SyntaxFlow filename filter into regexes that match
// the same strings as compilePathMatcher. ok is false when the conversion
// would widen or narrow the match; matchAll is the historical "*" / "*.*"
// short-circuit, which is wider than the glob "*.*".
func pathRegexes(expr string) ([]pathRegex, bool, bool) {
	if expr == "*" || expr == "*.*" {
		return nil, true, true
	}
	_, reErr := regexp.Compile(expr)
	_, globErr := glob.Compile(expr)
	reOK, globOK := reErr == nil, globErr == nil
	exact := "^" + regexp.QuoteMeta(expr) + "$"
	switch {
	case reOK && globOK:
		// Exact, unanchored regexp and glob are different languages here.
		return nil, false, false
	case reOK:
		return []pathRegex{
			{expr: exact, where: pathFull},
			{expr: expr, where: pathFull | pathBase},
		}, false, true
	case globOK:
		converted, ok := simpleGlobRegex(expr)
		if !ok {
			return nil, false, false
		}
		return []pathRegex{
			{expr: exact, where: pathFull},
			{expr: converted, where: pathFull | pathSlash | pathSlashBase},
		}, false, true
	default:
		return []pathRegex{{expr: exact, where: pathFull}}, false, true
	}
}

// simpleGlobRegex converts a separator-free gobwas glob of literals, * and ?.
// Character classes, braces and ** stay on compilePathMatcher.
func simpleGlobRegex(pattern string) (string, bool) {
	if strings.Contains(pattern, "**") || strings.ContainsAny(pattern, "[]{}") {
		return "", false
	}
	var b strings.Builder
	b.WriteByte('^')
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '\\':
			if i+1 >= len(pattern) {
				b.WriteString(regexp.QuoteMeta(`\`))
				break
			}
			i++
			b.WriteString(regexp.QuoteMeta(pattern[i : i+1]))
		case '*':
			// No separator was passed to glob.Compile, so * crosses '/'.
			b.WriteString("(?s:.*)")
		case '?':
			b.WriteString("(?s:.)")
		default:
			b.WriteString(regexp.QuoteMeta(pattern[i : i+1]))
		}
	}
	b.WriteByte('$')
	return b.String(), true
}
