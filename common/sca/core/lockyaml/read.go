// Package lockyaml reads the finite YAML data forms in pnpm v5/v6/v9 lock files.
// Mappings, sequences, quoted/plain scalars, and flow collections are supported.
// Tags, aliases, anchors, merge keys and multiple documents fail explicitly.
// No reflection codec, emitter, resolver schema, or filesystem IO is provided.
package lockyaml

import (
	"bytes"
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"strconv"
	"strings"
	"unicode/utf8"
)

type line struct {
	indent, number int
	text           string
}
type reader struct {
	limits     budget.Limits
	ctx        context.Context
	lines      []line
	pos, nodes int
}

func Parse(ctx context.Context, data []byte) (map[string]any, error) {
	if ctx != nil {
		ctx = budget.Ensure(ctx)
	}
	if ctx == nil {
		return nil, fmt.Errorf("nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	l := budget.From(ctx).Limits
	if int64(len(data)) > min(l.MaxFileBytes, 16<<20) {
		return nil, fmt.Errorf("resource_limit: YAML bytes")
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("malformed_input: YAML UTF-8")
	}
	if err := budget.From(ctx).Working(int64(len(data))); err != nil {
		return nil, err
	}
	st := budget.From(ctx)
	if err := st.Working(16*int64(bytes.Count(data, []byte{'\n'})+1) + budget.SizeSlice); err != nil {
		return nil, err
	}
	p := &reader{ctx: ctx, limits: l}
	var pending string
	start := 0
	pendingIndent := 0
	for i, raw := range strings.Split(strings.TrimPrefix(string(data), "\ufeff"), "\n") {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		raw = strings.TrimSuffix(raw, "\r")
		indent := len(raw) - len(strings.TrimLeft(raw, " "))
		if strings.ContainsRune(raw[:indent], '\t') || strings.HasPrefix(raw[indent:], "\t") {
			return nil, fmt.Errorf("unsupported_syntax: YAML tab indentation at %d", i+1)
		}
		if pending == "" {
			start = i + 1
			pendingIndent = indent
		}
		trimmed := strings.TrimSpace(raw)
		if len(trimmed) > l.MaxFieldBytes-len(pending) {
			return nil, fmt.Errorf("resource_limit: YAML flow field")
		}
		if err := st.Working(int64(len(pending) + len(trimmed))); err != nil {
			return nil, err
		}
		text, depth, quoted, err := clean(pending + trimmed)
		if err != nil {
			return nil, err
		}
		if quoted {
			return nil, fmt.Errorf("unsupported_syntax: multiline YAML quoted scalar at %d", start)
		}
		if depth > 0 {
			if err := st.Working(int64(len(text) + 1)); err != nil {
				return nil, err
			}
			pending = text + " "
			if len(pending) > l.MaxFieldBytes {
				return nil, fmt.Errorf("resource_limit: YAML flow field")
			}
			continue
		}
		pending = ""
		if text == "" {
			continue
		}
		if text == "---" || text == "..." || strings.HasPrefix(text, "%") {
			return nil, fmt.Errorf("unsupported_syntax: YAML document directive")
		}
		if err := budget.From(ctx).Result(budget.SizeObject + budget.SizeOfString(text)); err != nil {
			return nil, err
		}
		next, err := budget.Grow(st, p.lines, 1, 32)
		if err != nil {
			return nil, err
		}
		p.lines = append(next, line{pendingIndent, start, text})
		if len(p.lines) > l.MaxExpressionNodes {
			return nil, fmt.Errorf("resource_limit: YAML lines")
		}
	}
	if pending != "" {
		return nil, fmt.Errorf("malformed_input: unclosed YAML flow collection")
	}
	if len(p.lines) == 0 {
		return map[string]any{}, nil
	}
	if p.lines[0].indent != 0 {
		return nil, fmt.Errorf("malformed_input: YAML root indentation")
	}
	value, err := p.block(0, 0)
	if err != nil {
		return nil, err
	}
	m, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("malformed_input: lock root must be mapping")
	}
	if p.pos != len(p.lines) {
		return nil, fmt.Errorf("malformed_input: YAML trailing block")
	}
	return m, nil
}
func clean(s string) (string, int, bool, error) {
	depth := 0
	var quote byte
	escaped := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if quote == '"' && c == '\\' && !escaped {
				escaped = true
				continue
			}
			if c == quote && !escaped {
				if quote == '\'' && i+1 < len(s) && s[i+1] == '\'' {
					i++
					continue
				}
				quote = 0
			}
			escaped = false
			continue
		}
		// Quotes are indicators at the start of a key/value, not inside plain data.
		if (c == '"' || c == '\'') && (i == 0 || strings.ContainsRune(" [{,:-", rune(s[i-1]))) {
			quote = c
			continue
		}
		switch c {
		case '[', '{':
			depth++
		case ']', '}':
			depth--
			if depth < 0 {
				return "", 0, false, fmt.Errorf("malformed_input: YAML closing delimiter")
			}
		case '#':
			if i == 0 || s[i-1] == ' ' || s[i-1] == '\t' {
				return strings.TrimSpace(s[:i]), depth, false, nil
			}
		}
	}
	return strings.TrimSpace(s), depth, quote != 0, nil
}
func (p *reader) check(depth int) error {
	if err := p.ctx.Err(); err != nil {
		return err
	}
	if depth > p.limits.MaxSyntaxDepth {
		return fmt.Errorf("resource_limit: YAML nodes or depth")
	}
	return budget.From(p.ctx).Add(1, budget.SizeObject)
}
func seq(s string) bool { return s == "-" || strings.HasPrefix(s, "- ") }
func (p *reader) block(indent, depth int) (any, error) {
	if err := p.check(depth); err != nil {
		return nil, err
	}
	if p.pos >= len(p.lines) || p.lines[p.pos].indent != indent {
		return nil, fmt.Errorf("malformed_input: YAML block indentation")
	}
	if !seq(p.lines[p.pos].text) {
		return p.mapping(indent, depth, nil)
	}
	if err := budget.From(p.ctx).Result(budget.SizeSlice); err != nil {
		return nil, err
	}
	var list []any
	for p.pos < len(p.lines) && p.lines[p.pos].indent == indent && seq(p.lines[p.pos].text) {
		l := p.lines[p.pos]
		p.pos++
		rest := strings.TrimSpace(l.text[1:])
		var v any
		var err error
		if splitKey(rest) >= 0 {
			first := line{indent + 2, l.number, rest}
			v, err = p.mapping(indent+2, depth+1, &first)
		} else {
			v, err = p.value(rest, indent, depth+1)
		}
		if err != nil {
			return nil, err
		}
		next, err := budget.Grow(budget.From(p.ctx), list, 1, 16)
		if err != nil {
			return nil, err
		}
		list = append(next, v)
	}
	return list, nil
}
func (p *reader) mapping(indent, depth int, first *line) (map[string]any, error) {
	if err := p.check(depth); err != nil {
		return nil, err
	}
	if err := budget.From(p.ctx).Result(budget.SizeMap); err != nil {
		return nil, err
	}
	m := map[string]any{}
	for first != nil || p.pos < len(p.lines) && p.lines[p.pos].indent == indent {
		var l line
		if first != nil {
			l = *first
			first = nil
		} else {
			l = p.lines[p.pos]
			p.pos++
		}
		cut := splitKey(l.text)
		if cut < 0 {
			return nil, fmt.Errorf("unsupported_syntax: YAML mapping line %d", l.number)
		}
		key, err := scalar(strings.TrimSpace(l.text[:cut]))
		if err != nil {
			return nil, err
		}
		k, ok := key.(string)
		if !ok || k == "<<" {
			return nil, fmt.Errorf("unsupported_syntax: YAML key at %d", l.number)
		}
		if _, ok = m[k]; ok {
			return nil, fmt.Errorf("malformed_input: duplicate YAML key %q at %d", k, l.number)
		}
		v, err := p.value(strings.TrimSpace(l.text[cut+1:]), indent, depth+1)
		if err != nil {
			return nil, err
		}
		if err := budget.From(p.ctx).Insert(k); err != nil {
			return nil, err
		}
		m[k] = v
		if p.pos < len(p.lines) && p.lines[p.pos].indent > indent {
			return nil, fmt.Errorf("unsupported_syntax: YAML scalar continuation at %d", p.lines[p.pos].number)
		}
	}
	return m, nil
}
func splitKey(s string) int {
	var q byte
	escape := false
	depth := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if q != 0 {
			if q == '"' && c == '\\' && !escape {
				escape = true
				continue
			}
			if c == q && !escape {
				if q == '\'' && i+1 < len(s) && s[i+1] == '\'' {
					i++
					continue
				}
				q = 0
			}
			escape = false
			continue
		}
		if c == '"' || c == '\'' {
			q = c
			continue
		}
		switch c {
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		case ':':
			if depth == 0 && (i+1 == len(s) || s[i+1] == ' ' || s[i+1] == '\t') {
				return i
			}
		}
	}
	return -1
}
func (p *reader) value(s string, indent, depth int) (any, error) {
	if err := p.check(depth); err != nil {
		return nil, err
	}
	if s == "" {
		if p.pos < len(p.lines) && p.lines[p.pos].indent > indent {
			return p.block(p.lines[p.pos].indent, depth+1)
		}
		return nil, nil
	}
	if len(s) > p.limits.MaxFieldBytes {
		return nil, fmt.Errorf("resource_limit: YAML scalar bytes")
	}
	if s[0] == '{' || s[0] == '[' {
		f := flow{p: p, s: s}
		v, e := f.value(depth)
		f.space()
		if e == nil && f.i != len(s) {
			e = fmt.Errorf("malformed_input: YAML flow trailing text")
		}
		return v, e
	}
	return scalar(s)
}
func scalar(s string) (any, error) {
	if s == "" || s == "null" || s == "Null" || s == "NULL" || s == "~" {
		return nil, nil
	}
	if s[0] == '\'' {
		if len(s) < 2 || s[len(s)-1] != '\'' {
			return nil, fmt.Errorf("malformed_input: YAML quote")
		}
		return strings.ReplaceAll(s[1:len(s)-1], "''", "'"), nil
	}
	if s[0] == '"' {
		return double(s)
	}
	if strings.ContainsAny(s[:1], "&*!|>@`%") || s == "?" || s == "-" || strings.Contains(s, ": ") {
		return nil, fmt.Errorf("unsupported_syntax: YAML scalar %q", s)
	}
	switch s {
	case "true", "True", "TRUE":
		return true, nil
	case "false", "False", "FALSE":
		return false, nil
	}
	// Numeric-looking identifiers deliberately remain exact strings.
	return s, nil
}
func double(s string) (string, error) {
	if len(s) < 2 || s[len(s)-1] != '"' {
		return "", fmt.Errorf("malformed_input: YAML quote")
	}
	var b strings.Builder
	for i := 1; i < len(s)-1; i++ {
		c := s[i]
		if c == '"' {
			return "", fmt.Errorf("malformed_input: YAML quote suffix")
		}
		if c != '\\' {
			b.WriteByte(c)
			continue
		}
		i++
		if i >= len(s)-1 {
			return "", fmt.Errorf("malformed_input: YAML escape")
		}
		escapes := map[byte]rune{'0': 0, 'a': 7, 'b': 8, 't': 9, 'n': 10, 'v': 11, 'f': 12, 'r': 13, 'e': 27, ' ': 32, '"': 34, '/': 47, '\\': 92, 'N': 0x85, '_': 0xa0, 'L': 0x2028, 'P': 0x2029}
		if r, ok := escapes[s[i]]; ok {
			b.WriteRune(r)
			continue
		}
		digits := 0
		switch s[i] {
		case 'x':
			digits = 2
		case 'u':
			digits = 4
		case 'U':
			digits = 8
		default:
			return "", fmt.Errorf("malformed_input: YAML escape")
		}
		if i+digits >= len(s)-1 {
			return "", fmt.Errorf("malformed_input: YAML unicode escape")
		}
		v, e := strconv.ParseUint(s[i+1:i+digits+1], 16, 32)
		if e != nil || !utf8.ValidRune(rune(v)) {
			return "", fmt.Errorf("malformed_input: YAML unicode scalar")
		}
		b.WriteRune(rune(v))
		i += digits
	}
	return b.String(), nil
}

type flow struct {
	p *reader
	s string
	i int
}

func (f *flow) space() {
	for f.i < len(f.s) && (f.s[f.i] == ' ' || f.s[f.i] == '\t') {
		f.i++
	}
}
func (f *flow) value(depth int) (any, error) {
	if err := f.p.check(depth); err != nil {
		return nil, err
	}
	f.space()
	if f.i >= len(f.s) {
		return nil, fmt.Errorf("malformed_input: YAML flow value")
	}
	c := f.s[f.i]
	if c == '{' || c == '[' {
		f.i++
		if c == '{' {
			if err := budget.From(f.p.ctx).Result(budget.SizeMap); err != nil {
				return nil, err
			}
		} else if err := budget.From(f.p.ctx).Result(budget.SizeSlice); err != nil {
			return nil, err
		}
		m := map[string]any{}
		var a []any
		end := byte(']')
		if c == '{' {
			end = '}'
		}
		for {
			f.space()
			if f.i >= len(f.s) {
				return nil, fmt.Errorf("malformed_input: unclosed YAML flow")
			}
			if f.s[f.i] == end {
				f.i++
				if c == '{' {
					return m, nil
				}
				return a, nil
			}
			key := ""
			if c == '{' {
				start := f.i
				var q byte
				esc := false
				for f.i < len(f.s) {
					x := f.s[f.i]
					if q != 0 {
						if x == '\\' && q == '"' && !esc {
							esc = true
							f.i++
							continue
						}
						if x == q && !esc {
							q = 0
						}
						esc = false
					} else {
						if x == '\'' || x == '"' {
							q = x
						} else if x == ':' && (f.i+1 == len(f.s) || strings.ContainsRune(" \t[{\"'", rune(f.s[f.i+1]))) {
							break
						}
					}
					f.i++
				}
				if f.i >= len(f.s) {
					return nil, fmt.Errorf("malformed_input: YAML flow key")
				}
				v, err := scalar(strings.TrimSpace(f.s[start:f.i]))
				if err != nil {
					return nil, err
				}
				var ok bool
				key, ok = v.(string)
				if !ok || key == "<<" {
					return nil, fmt.Errorf("unsupported_syntax: YAML flow key")
				}
				if _, ok = m[key]; ok {
					return nil, fmt.Errorf("malformed_input: duplicate YAML flow key")
				}
				f.i++
			}
			v, err := f.value(depth + 1)
			if err != nil {
				return nil, err
			}
			if c == '{' {
				if err := budget.From(f.p.ctx).Insert(key); err != nil {
					return nil, err
				}
				m[key] = v
			} else {
				next, err := budget.Grow(budget.From(f.p.ctx), a, 1, 16)
				if err != nil {
					return nil, err
				}
				a = append(next, v)
			}
			f.space()
			if f.i >= len(f.s) {
				return nil, fmt.Errorf("malformed_input: YAML flow end")
			}
			if f.s[f.i] == ',' {
				f.i++
				continue
			}
			if f.s[f.i] != end {
				return nil, fmt.Errorf("malformed_input: YAML flow separator")
			}
		}
	}
	start := f.i
	if c == '"' || c == '\'' {
		f.i++
		closed := false
		for f.i < len(f.s) {
			x := f.s[f.i]
			f.i++
			if x == '\\' && c == '"' {
				f.i++
				continue
			}
			if x == c {
				if c == '\'' && f.i < len(f.s) && f.s[f.i] == '\'' {
					f.i++
					continue
				}
				closed = true
				break
			}
		}
		if !closed || f.i > len(f.s) {
			return nil, fmt.Errorf("malformed_input: YAML flow quote")
		}
	} else {
		for f.i < len(f.s) && !strings.ContainsRune(",]}", rune(f.s[f.i])) {
			f.i++
		}
	}
	if f.i == start {
		return nil, fmt.Errorf("malformed_input: empty YAML flow value")
	}
	return scalar(strings.TrimSpace(f.s[start:f.i]))
}
