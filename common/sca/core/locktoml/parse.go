package locktoml

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"strconv"
	"strings"
)

type tableOrigin uint8

const (
	implicit tableOrigin = iota
	dotted
	explicit
)

type node struct {
	startLine, endLine int
	table              map[string]*node
	array              []*node
	scalar             any
	origin             tableOrigin
	sealed, aot        bool
}

type parser struct {
	lastLine      int
	limits        budget.Limits
	lx            *lexer
	tokens        int
	mapping       map[string]any
	root, current *node
}

func (p *parser) table(origin tableOrigin) *node {
	if err := budget.From(p.lx.ctx).Add(1, budget.SizeMap+budget.SizeObject); err != nil {
		panic(parseAbort{err})
	}
	return &node{table: map[string]*node{}, origin: origin}
}

func parse(ctx context.Context, data string) (p *parser, err error) {
	defer func() {
		if v := recover(); v != nil {
			if a, ok := v.(parseAbort); ok {
				err = a.err
			} else {
				panic(v)
			}
		}
	}()
	p = &parser{limits: budget.From(ctx).Limits, lx: lex(strings.TrimPrefix(data, "\ufeff"))}
	p.lx.ctx = ctx
	p.root = p.table(explicit)
	p.current = p.root
	for {
		it := p.next()
		switch it.typ {
		case itemEOF:
			p.mapping = p.root.value(budget.From(ctx)).(map[string]any)
			return p, nil
		case itemCommentStart:
			p.expect(itemText)
		case itemTableStart, itemArrayTableStart:
			end := itemTableEnd
			if it.typ == itemArrayTableStart {
				end = itemArrayTableEnd
			}
			key := p.key(end)
			p.current = p.header(key, it.typ == itemArrayTableStart)
			p.current.startLine = it.pos.Line
			p.current.endLine = p.lastLine
		case itemKeyStart:
			key := p.key(itemKeyEnd)
			v := p.value(p.next(), 0)
			p.assign(p.current, key, v)
			p.current.endLine = p.lastLine
		default:
			p.panicItemf(it, "unexpected token at top level")
		}
	}
}
func (p *parser) next() item {
	if err := budget.From(p.lx.ctx).Add(1, budget.SizeObject); err != nil {
		panic(parseAbort{err})
	}
	it := p.lx.nextItem()
	p.lastLine = it.pos.Line + strings.Count(it.val, "\n")
	if len(it.val) > p.limits.MaxFieldBytes {
		panic(parseAbort{fmt.Errorf("resource_limit: TOML token bytes")})
	}
	if it.typ == itemError {
		if it.err != nil {
			p.panicItemf(it, "%v", it.err)
		}
		p.panicItemf(it, "%s", it.val)
	}
	return it
}
func (p *parser) expect(typ itemType) item {
	it := p.next()
	if it.typ != typ {
		p.panicItemf(it, "unexpected token")
	}
	return it
}
func (p *parser) panicItemf(it item, format string, args ...any) {
	panic(parseAbort{fmt.Errorf("malformed_input: TOML line %d: %s", it.pos.Line, fmt.Sprintf(format, args...))})
}
func (p *parser) bug(format string, args ...any) {
	panic(parseAbort{fmt.Errorf("malformed_input: TOML: %s", fmt.Sprintf(format, args...))})
}
func (p *parser) key(end itemType) []string {
	var key []string
	for {
		it := p.next()
		if it.typ == end {
			break
		}
		var s string
		switch it.typ {
		case itemText:
			s = it.val
		case itemString:
			s = p.replaceEscapes(it, it.val)
		case itemRawString:
			s = it.val
		default:
			p.panicItemf(it, "invalid key")
		}
		nextKey, err := budget.Grow(budget.From(p.lx.ctx), key, 1, budget.SizeString)
		if err != nil {
			panic(parseAbort{err})
		}
		key = append(nextKey, s)
		if len(key) > p.limits.MaxSyntaxDepth {
			panic(parseAbort{fmt.Errorf("resource_limit: TOML key depth")})
		}
	}
	if len(key) == 0 {
		p.bug("empty key")
	}
	return key
}
func (p *parser) header(key []string, array bool) *node {
	n := p.root
	for _, k := range key[:len(key)-1] {
		v := n.table[k]
		if v == nil {
			v = p.table(implicit)
			if err := budget.From(p.lx.ctx).Insert(k); err != nil {
				panic(parseAbort{err})
			}
			n.table[k] = v
		}
		if v.aot {
			v = v.array[len(v.array)-1]
		}
		if v.table == nil || v.sealed {
			p.bug("table crosses a value or inline table")
		}
		n = v
	}
	k := key[len(key)-1]
	v := n.table[k]
	if array {
		if v == nil {
			if err := budget.From(p.lx.ctx).Add(1, budget.SizeSlice+budget.SizeObject); err != nil {
				panic(parseAbort{err})
			}
			v = &node{aot: true}
			if err := budget.From(p.lx.ctx).Insert(k); err != nil {
				panic(parseAbort{err})
			}
			n.table[k] = v
		}
		if !v.aot {
			p.bug("array table redefines key")
		}
		next := p.table(explicit)
		grown, err := budget.Grow(budget.From(p.lx.ctx), v.array, 1, budget.SizePtr)
		if err != nil {
			panic(parseAbort{err})
		}
		v.array = append(grown, next)
		return next
	}
	if v == nil {
		v = p.table(explicit)
		if err := budget.From(p.lx.ctx).Insert(k); err != nil {
			panic(parseAbort{err})
		}
		n.table[k] = v
		return v
	}
	if v.table == nil || v.sealed || v.origin != implicit {
		p.bug("table already defined")
	}
	v.origin = explicit
	return v
}
func (p *parser) assign(n *node, key []string, v *node) {
	for _, k := range key[:len(key)-1] {
		next := n.table[k]
		if next == nil {
			next = p.table(dotted)
			if err := budget.From(p.lx.ctx).Insert(k); err != nil {
				panic(parseAbort{err})
			}
			n.table[k] = next
		}
		if next.table == nil || next.sealed || next.origin == explicit {
			p.bug("dotted key extends a closed or explicit table")
		}
		n = next
	}
	k := key[len(key)-1]
	if n.table[k] != nil {
		p.bug("duplicate key %q", k)
	}
	if err := budget.From(p.lx.ctx).Insert(k); err != nil {
		panic(parseAbort{err})
	}
	n.table[k] = v
}
func (p *parser) value(it item, depth int) *node {
	if depth > p.limits.MaxSyntaxDepth {
		panic(parseAbort{fmt.Errorf("resource_limit: TOML value depth")})
	}
	n := &node{}
	switch it.typ {
	case itemString:
		n.scalar = p.replaceEscapes(it, it.val)
	case itemRawString:
		n.scalar = it.val
	case itemMultilineString:
		n.scalar = p.replaceEscapes(it, p.stripEscapedNewlines(stripFirstNewline(it.val)))
	case itemRawMultilineString:
		n.scalar = stripFirstNewline(it.val)
	case itemBool:
		n.scalar = it.val == "true"
	case itemInteger:
		if !numUnderscoresOK(it.val) || numHasLeadingZero(it.val) {
			p.panicItemf(it, "invalid integer")
		}
		v, e := strconv.ParseInt(it.val, 0, 64)
		if e != nil {
			p.panicItemf(it, "invalid integer: %v", e)
		}
		n.scalar = v
	case itemFloat:
		parts := strings.FieldsFunc(it.val, func(r rune) bool { return r == '.' || r == 'e' || r == 'E' })
		for _, part := range parts {
			if !numUnderscoresOK(part) {
				p.panicItemf(it, "invalid float")
			}
		}
		if len(parts) > 0 && numHasLeadingZero(parts[0]) || !numPeriodsOK(it.val) {
			p.panicItemf(it, "invalid float")
		}
		s := strings.ReplaceAll(it.val, "_", "")
		if s == "+nan" || s == "-nan" {
			s = "nan"
		}
		v, e := strconv.ParseFloat(s, 64)
		if e != nil {
			p.panicItemf(it, "invalid float: %v", e)
		}
		n.scalar = v
	case itemDatetime:
		panic(parseAbort{fmt.Errorf("unsupported_syntax: dates in SCA lock files")})
	case itemArray:
		if err := budget.From(p.lx.ctx).Result(budget.SizeSlice); err != nil {
			panic(parseAbort{err})
		}
		n.array = []*node{}
		for {
			next := p.next()
			if next.typ == itemArrayEnd {
				break
			}
			if next.typ == itemCommentStart {
				p.expect(itemText)
				continue
			}
			item := p.value(next, depth+1)
			grown, err := budget.Grow(budget.From(p.lx.ctx), n.array, 1, budget.SizePtr)
			if err != nil {
				panic(parseAbort{err})
			}
			n.array = append(grown, item)
		}
	case itemInlineTableStart:
		n = p.table(dotted)
		for {
			next := p.next()
			if next.typ == itemInlineTableEnd {
				break
			}
			if next.typ != itemKeyStart {
				p.panicItemf(next, "invalid inline table key")
			}
			key := p.key(itemKeyEnd)
			p.assign(n, key, p.value(p.next(), depth+1))
		}
		n.sealed = true
	default:
		p.panicItemf(it, "unexpected value")
	}
	return n
}
func (n *node) value(st *budget.State) any {
	// Reserve each additional map/interface array before materializing it.
	charge := func(count int, unit, header int64) {
		size, err := budget.SizeMul(count, unit)
		if err == nil {
			size, err = budget.SizeAdd(size, header)
		}
		if err == nil {
			err = st.Working(size)
		}
		if err != nil {
			panic(parseAbort{err})
		}
	}

	if n.table != nil {
		charge(len(n.table), budget.SizeString+2*budget.SizePtr+budget.SizeObject, budget.SizeMap)
		m := make(map[string]any, len(n.table))
		for k, v := range n.table {
			m[k] = v.value(st)
		}
		return m
	}
	if n.array != nil {
		charge(len(n.array), 2*budget.SizePtr, budget.SizeSlice)
		a := make([]any, len(n.array))
		for i, v := range n.array {
			a[i] = v.value(st)
		}
		return a
	}
	return n.scalar
}
