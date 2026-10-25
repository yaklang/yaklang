package pcaputil

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"unicode"
)

type DisplayFilter struct{ match func(map[string][]any) bool }

type DisplayFilterResult struct {
	Matched  bool   `json:"matched"`
	Complete bool   `json:"complete"`
	Reason   string `json:"reason,omitempty"`
}

// Match evaluates one PDU. MatchEvents evaluates the union of fields from a
// caller-supplied packet's PDUs, never from unrelated connections or packets.
// A truncated projection is indeterminate and must not produce a positive match.
func (f *DisplayFilter) Match(e *ProtocolEvent) bool {
	if f == nil {
		return true
	}
	return f.MatchEvents([]*ProtocolEvent{e})
}

func (f *DisplayFilter) MatchEvents(events []*ProtocolEvent) bool {
	result := f.EvaluateEvents(events)
	return result.Complete && result.Matched
}

func (f *DisplayFilter) EvaluateEvents(events []*ProtocolEvent) DisplayFilterResult {
	if f == nil {
		return DisplayFilterResult{Matched: true, Complete: true}
	}
	m := map[string][]any{}
	count := 0
	for _, e := range events {
		if e != nil && (e.Completeness == "evicted" || e.Completeness == "limited") {
			return DisplayFilterResult{Reason: "protocol detail unavailable in retained history"}
		}
		snapshot := e.DisplayFieldValues()
		if snapshot.Truncated {
			return DisplayFilterResult{Reason: "field projection budget exceeded"}
		}
		if snapshot.Incomplete {
			return DisplayFilterResult{Reason: "field value cannot be represented by its registered type"}
		}
		for _, v := range snapshot.Values {
			count++
			if count > maxDisplayFieldValues {
				return DisplayFilterResult{Reason: "packet field union budget exceeded"}
			}
			m[v.Name] = append(m[v.Name], v.Value)
		}
	}
	return DisplayFilterResult{Matched: f.match(m), Complete: true}
}

type filterToken struct {
	s      string
	quoted bool
}

func CompileDisplayFilter(expr string) (*DisplayFilter, error) {
	if len(expr) > 4096 {
		return nil, fmt.Errorf("display filter exceeds 4096 bytes")
	}
	var tokens []filterToken
	for i := 0; i < len(expr); {
		if unicode.IsSpace(rune(expr[i])) {
			i++
			continue
		}
		if len(tokens) >= 128 {
			return nil, fmt.Errorf("display filter token budget")
		}
		if expr[i] == '"' {
			start := i
			i++
			for i < len(expr) && expr[i] != '"' {
				if expr[i] == '\\' {
					i++
				}
				i++
			}
			if i >= len(expr) {
				return nil, fmt.Errorf("unterminated filter string")
			}
			i++
			s, err := strconv.Unquote(expr[start:i])
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, filterToken{s, true})
			continue
		}
		if strings.ContainsRune("()=!<>", rune(expr[i])) {
			n := 1
			if i+1 < len(expr) && expr[i+1] == '=' && expr[i] != '(' && expr[i] != ')' {
				n = 2
			}
			tokens = append(tokens, filterToken{s: expr[i : i+n]})
			i += n
			continue
		}
		start := i
		for i < len(expr) && !unicode.IsSpace(rune(expr[i])) && !strings.ContainsRune("()=!<>\"", rune(expr[i])) {
			i++
		}
		tokens = append(tokens, filterToken{s: expr[start:i]})
	}
	if len(tokens) == 0 {
		return nil, nil
	}
	p := filterParser{tokens: tokens}
	fn, err := p.parse(0, 0)
	if err != nil {
		return nil, err
	}
	if p.pos != len(tokens) {
		return nil, fmt.Errorf("unexpected filter token")
	}
	return &DisplayFilter{fn}, nil
}

type filterParser struct {
	tokens []filterToken
	pos    int
}
type fieldPredicate func(map[string][]any) bool

func (p *filterParser) take(s string) bool {
	if p.pos < len(p.tokens) && !p.tokens[p.pos].quoted && p.tokens[p.pos].s == s {
		p.pos++
		return true
	}
	return false
}
func (p *filterParser) parse(level, depth int) (fieldPredicate, error) {
	if depth > 32 {
		return nil, fmt.Errorf("display filter depth")
	}
	if level < 2 {
		left, err := p.parse(level+1, depth+1)
		if err != nil {
			return nil, err
		}
		op := "or"
		if level == 1 {
			op = "and"
		}
		for p.take(op) {
			right, err := p.parse(level+1, depth+1)
			if err != nil {
				return nil, err
			}
			a, b := left, right
			if op == "or" {
				left = func(m map[string][]any) bool { return a(m) || b(m) }
			} else {
				left = func(m map[string][]any) bool { return a(m) && b(m) }
			}
		}
		return left, nil
	}
	if p.take("not") {
		f, err := p.parse(2, depth+1)
		return func(m map[string][]any) bool { return !f(m) }, err
	}
	if p.take("(") {
		f, err := p.parse(0, depth+1)
		if err != nil {
			return nil, err
		}
		if !p.take(")") {
			return nil, fmt.Errorf("missing closing parenthesis")
		}
		return f, nil
	}
	if p.pos >= len(p.tokens) {
		return nil, fmt.Errorf("missing field")
	}
	name := p.tokens[p.pos].s
	quoted := p.tokens[p.pos].quoted
	p.pos++
	descriptor, registered := LookupDisplayField(name)
	if quoted || !registered {
		return nil, fmt.Errorf("unknown display field %q", name)
	}
	op := ""
	for _, candidate := range []string{"==", "!=", ">=", "<=", ">", "<", "contains", "in"} {
		if p.take(candidate) {
			op = candidate
			break
		}
	}
	if op == "" {
		return func(m map[string][]any) bool { return len(m[name]) > 0 }, nil
	}
	if p.pos >= len(p.tokens) {
		return nil, fmt.Errorf("missing comparison value")
	}
	value := p.tokens[p.pos]
	p.pos++
	literal, err := validateDisplayFieldLiteral(descriptor, op, value)
	if err != nil {
		return nil, err
	}
	return func(m map[string][]any) bool {
		vals := m[name]
		if len(vals) == 0 {
			return false
		}
		equal := false
		for _, v := range vals {
			if op == "in" {
				addr, err := netip.ParseAddr(fmt.Sprint(v))
				if err == nil && literal.(netip.Prefix).Contains(addr.Unmap()) {
					return true
				}
				continue
			}
			if op == "contains" {
				if text, ok := v.(string); ok && strings.Contains(text, literal.(string)) {
					return true
				}
				continue
			}
			cmp, valid := compareDisplayValue(v, literal, descriptor)
			if !valid {
				continue
			}
			if cmp == 0 {
				equal = true
			}
			switch op {
			case "==":
				if cmp == 0 {
					return true
				}
			case ">":
				if cmp > 0 {
					return true
				}
			case "<":
				if cmp < 0 {
					return true
				}
			case ">=":
				if cmp >= 0 {
					return true
				}
			case "<=":
				if cmp <= 0 {
					return true
				}
			}
		}
		return op == "!=" && !equal
	}, nil
}

func compareDisplayValue(actual, literal any, descriptor DisplayFieldDescriptor) (int, bool) {
	value, ok := normalizeDisplayValue(actual, descriptor)
	if !ok {
		return 0, false
	}
	switch descriptor.Type {
	case DisplayUnsigned:
		a, b := value.(uint64), literal.(uint64)
		if a < b {
			return -1, true
		}
		if a > b {
			return 1, true
		}
		return 0, true
	case DisplayString:
		return strings.Compare(value.(string), literal.(string)), true
	case DisplayBoolean:
		if value.(bool) == literal.(bool) {
			return 0, true
		}
		return 1, true
	case DisplayAddress:
		a, err := netip.ParseAddr(value.(string))
		if err != nil {
			return 0, false
		}
		return a.Unmap().Compare(literal.(netip.Addr)), true
	}
	return 0, false
}
