package pcaputil

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"unicode"
)

type DisplayFilter struct{ match fieldPredicate }

type DisplayFilterResult struct {
	Matched bool `json:"matched"`
	// Complete certifies the predicate's outcome, not an exhaustive projection.
	Complete bool   `json:"complete"`
	Partial  bool   `json:"partial,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// Match evaluates one PDU. MatchEvents evaluates the union of fields from a
// caller-supplied packet's PDUs, never from unrelated connections or packets.
// Retained witnesses can decide a predicate on a partial projection. Missing
// fields cannot prove absence, and universal comparisons require complete input.
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
	return f.evaluateSnapshots(len(events), func(i int) DisplayFieldSnapshot {
		e := events[i]
		if e != nil && e.Completeness == "evicted" {
			return DisplayFieldSnapshot{Completeness: e.Completeness}
		}
		return e.DisplayFieldValues()
	})
}

// EvaluateSnapshots evaluates an owned packet observation without consulting
// mutable history or reparsing a PDU. Snapshots must belong to the same packet;
// completeness and projection limits remain part of the filtering contract.
func (f *DisplayFilter) EvaluateSnapshots(snapshots []DisplayFieldSnapshot) DisplayFilterResult {
	return f.evaluateSnapshots(len(snapshots), func(i int) DisplayFieldSnapshot { return snapshots[i] })
}

func (f *DisplayFilter) evaluateSnapshots(length int, at func(int) DisplayFieldSnapshot) DisplayFilterResult {
	if f == nil {
		return DisplayFilterResult{Matched: true, Complete: true}
	}
	m := fieldObservation{values: map[string][]any{}, exhaustive: true}
	reason := ""
	partial := func(s string) {
		m.exhaustive = false
		if reason == "" {
			reason = s
		}
	}
	count := 0
snapshots:
	for i := 0; i < length; i++ {
		snapshot := at(i)
		if snapshot.Completeness == "evicted" || snapshot.Completeness == "limited" {
			partial("protocol detail unavailable in retained history")
			if snapshot.Completeness == "evicted" {
				continue
			}
		}
		if snapshot.Truncated {
			partial("field projection budget exceeded")
		}
		if snapshot.Incomplete {
			partial("field projection incomplete or value not representable by its registered type")
		}
		for _, v := range snapshot.Values {
			count++
			if count > maxDisplayFieldValues {
				partial("packet field union budget exceeded")
				break snapshots
			}
			d, ok := LookupDisplayField(v.Name)
			if !ok {
				partial("unregistered snapshot field")
				continue
			}
			value := v.Value
			if v.ExactValue != "" {
				if d.Type == DisplayUnsigned && d.Bits > 53 {
					n, err := strconv.ParseUint(v.ExactValue, 10, d.Bits)
					if err != nil {
						partial("invalid exact unsigned field value")
						continue
					}
					value = n
				}
			}
			value, ok = normalizeDisplayValue(value, d)
			if !ok {
				partial("snapshot value not representable by its registered type")
				continue
			}
			m.values[v.Name] = append(m.values[v.Name], value)
		}
	}
	decision := f.match(m)
	return DisplayFilterResult{Matched: decision == fieldTrue, Complete: decision != fieldUnknown, Partial: !m.exhaustive, Reason: reason}
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
type fieldTruth uint8

const (
	fieldUnknown fieldTruth = iota
	fieldFalse
	fieldTrue
)

type fieldObservation struct {
	values     map[string][]any
	exhaustive bool
}

type fieldPredicate func(fieldObservation) fieldTruth

func fieldNot(t fieldTruth) fieldTruth {
	if t == fieldUnknown {
		return t
	}
	if t == fieldTrue {
		return fieldFalse
	}
	return fieldTrue
}

func fieldAnd(a, b fieldTruth) fieldTruth {
	if a == fieldFalse || b == fieldFalse {
		return fieldFalse
	}
	if a == fieldTrue && b == fieldTrue {
		return fieldTrue
	}
	return fieldUnknown
}

func fieldOr(a, b fieldTruth) fieldTruth {
	if a == fieldTrue || b == fieldTrue {
		return fieldTrue
	}
	if a == fieldFalse && b == fieldFalse {
		return fieldFalse
	}
	return fieldUnknown
}

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
				left = func(m fieldObservation) fieldTruth { return fieldOr(a(m), b(m)) }
			} else {
				left = func(m fieldObservation) fieldTruth { return fieldAnd(a(m), b(m)) }
			}
		}
		return left, nil
	}
	if p.take("not") {
		f, err := p.parse(2, depth+1)
		return func(m fieldObservation) fieldTruth { return fieldNot(f(m)) }, err
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
		return func(m fieldObservation) fieldTruth {
			if len(m.values[name]) > 0 {
				return fieldTrue
			}
			if m.exhaustive {
				return fieldFalse
			}
			return fieldUnknown
		}, nil
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
	return func(m fieldObservation) fieldTruth {
		vals := m.values[name]
		if len(vals) == 0 {
			if m.exhaustive {
				return fieldFalse
			}
			return fieldUnknown
		}
		equal := false
		for _, v := range vals {
			if op == "in" {
				addr, err := netip.ParseAddr(fmt.Sprint(v))
				if descriptor.Bits != 128 {
					addr = addr.Unmap()
				}
				if err == nil && literal.(netip.Prefix).Contains(addr) {
					return fieldTrue
				}
				continue
			}
			if op == "contains" {
				if text, ok := v.(string); ok && strings.Contains(text, literal.(string)) {
					return fieldTrue
				}
				continue
			}
			cmp, valid := compareDisplayValue(v, literal, descriptor)
			if !valid {
				continue
			}
			if cmp == 0 {
				equal = true
				if op == "!=" {
					return fieldFalse
				}
			}
			switch op {
			case "==":
				if cmp == 0 {
					return fieldTrue
				}
			case ">":
				if cmp > 0 {
					return fieldTrue
				}
			case "<":
				if cmp < 0 {
					return fieldTrue
				}
			case ">=":
				if cmp >= 0 {
					return fieldTrue
				}
			case "<=":
				if cmp <= 0 {
					return fieldTrue
				}
			}
		}
		if !m.exhaustive {
			return fieldUnknown
		}
		if op == "!=" && !equal {
			return fieldTrue
		}
		return fieldFalse
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
		if descriptor.Bits != 128 {
			a = a.Unmap()
		}
		return a.Compare(literal.(netip.Addr)), true
	}
	return 0, false
}
