// Package jsonrecord reads bounded JSON records and their source locations.
// The standard decoder owns JSON grammar. There are no reflection callbacks,
// custom object codecs, numeric coercions, or filesystem operations here.
package jsonrecord

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/scanerr"
	"io"
	"sort"
	"unicode/utf8"
)

type Node struct {
	Object             map[string]*Node
	Array              []*Node
	Raw                json.RawMessage
	StartLine, EndLine int
}

func (n *Node) Get(keys ...string) *Node {
	for _, k := range keys {
		if n == nil {
			return nil
		}
		n = n.Object[k]
	}
	return n
}
func (n *Node) Lines() (int, int) {
	if n == nil {
		return 0, 0
	}
	return n.StartLine, n.EndLine
}
func (n *Node) Elements() []*Node {
	if n == nil {
		return nil
	}
	return n.Array
}

type reader struct {
	ctx    context.Context
	raw    []byte
	dec    *json.Decoder
	lines  []int
	nodes  int
	limits budget.Limits
}

const MaxBytes = 16 << 20

// Parse rejects duplicate decoded keys, invalid UTF-8, trailing data, and
// excessive nesting before a caller performs its format-specific conversion.
func Parse(ctx context.Context, raw []byte) (*Node, error) {
	if ctx != nil {
		ctx = budget.Ensure(ctx)
	}
	if ctx == nil {
		return nil, fmt.Errorf("nil JSON context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	l := budget.From(ctx).Limits
	if int64(len(raw)) > min(int64(MaxBytes), l.MaxFileBytes) {
		return nil, fmt.Errorf("resource_limit: JSON bytes exceed %d", MaxBytes)
	}
	if !utf8.Valid(raw) {
		return nil, fmt.Errorf("malformed_input: JSON is not UTF-8")
	}
	if err := budget.From(ctx).Working(8 * int64(len(raw))); err != nil {
		return nil, err
	}
	if err := budget.From(ctx).Result(budget.SizeDecoderScratch); err != nil {
		return nil, err
	}
	newlines := bytes.Count(raw, []byte{'\n'})
	if err := budget.From(ctx).Result(budget.SizeSlice + int64(newlines)*8); err != nil {
		return nil, err
	}
	p := &reader{limits: l, ctx: ctx, raw: raw, dec: json.NewDecoder(bytes.NewReader(raw)), lines: make([]int, 0, newlines)}
	p.dec.UseNumber()
	for i, b := range raw {
		if b == '\n' {
			p.lines = append(p.lines, i)
		}
	}
	n, err := p.value(0)
	if err != nil {
		return nil, err
	}
	if _, err = p.token(); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("trailing JSON data")
		}
		if scanerr.CodeOf(err) != "" {
			return nil, err
		}
		return nil, fmt.Errorf("malformed_input: %w", err)
	}
	return n, nil
}
func (p *reader) value(depth int) (*Node, error) {
	if err := p.ctx.Err(); err != nil {
		return nil, err
	}
	if depth > p.limits.MaxSyntaxDepth {
		return nil, fmt.Errorf("resource_limit: JSON depth or nodes")
	}
	if err := budget.From(p.ctx).Add(1, budget.SizeOfNode(0)); err != nil {
		return nil, err
	}
	start := int(p.dec.InputOffset())
	for start < len(p.raw) && (p.raw[start] == ' ' || p.raw[start] == '\n' || p.raw[start] == '\r' || p.raw[start] == '\t' || p.raw[start] == ':' || p.raw[start] == ',') {
		start++
	}
	tok, err := p.token()
	if err != nil {
		if scanerr.CodeOf(err) != "" {
			return nil, err
		}
		return nil, fmt.Errorf("malformed_input: %w", err)
	}
	n := &Node{}
	if d, ok := tok.(json.Delim); ok {
		switch d {
		case '{':
			n.Object = map[string]*Node{}
			for p.dec.More() {
				key, err := p.token()
				if err != nil {
					return nil, err
				}
				k, ok := key.(string)
				if !ok {
					return nil, fmt.Errorf("malformed_input: object key")
				}
				if _, ok = n.Object[k]; ok {
					return nil, fmt.Errorf("malformed_input: duplicate JSON key %q", k)
				}
				v, err := p.value(depth + 1)
				if err != nil {
					return nil, err
				}
				if err := budget.From(p.ctx).Insert(k); err != nil {
					return nil, err
				}
				n.Object[k] = v
			}
			end, err := p.dec.Token()
			if err != nil || end != json.Delim('}') {
				return nil, fmt.Errorf("malformed_input: unclosed object: %v", err)
			}
		case '[':
			for p.dec.More() {
				v, err := p.value(depth + 1)
				if err != nil {
					return nil, err
				}
				next, err := budget.Grow(budget.From(p.ctx), n.Array, 1, budget.SizePtr)
				if err != nil {
					return nil, err
				}
				n.Array = append(next, v)
			}
			end, err := p.dec.Token()
			if err != nil || end != json.Delim(']') {
				return nil, fmt.Errorf("malformed_input: unclosed array: %v", err)
			}
		default:
			return nil, fmt.Errorf("malformed_input: unexpected delimiter")
		}
	}
	end := int(p.dec.InputOffset())
	if n.Object == nil && n.Array == nil && end-start > p.limits.MaxFieldBytes {
		return nil, scanerr.New(scanerr.ResourceLimit, "JSON scalar")
	}
	n.Raw = p.raw[start:end]
	n.StartLine = sort.SearchInts(p.lines, start) + 1
	n.EndLine = sort.SearchInts(p.lines, end-1) + 1
	return n, nil
}

// Decode validates the complete document, then decodes only into the caller's
// fixed record type. The returned nodes are used explicitly for field locations.
func Decode(ctx context.Context, raw []byte, record any) (*Node, error) {
	if ctx != nil {
		ctx = budget.Ensure(ctx)
	}
	n, err := Parse(ctx, raw)
	if err != nil {
		return nil, err
	}
	if err = budget.From(ctx).Result(budget.SizeDecoderScratch); err != nil {
		return n, err
	}
	if err = reserveRecord(budget.From(ctx), n); err != nil {
		return n, err
	}
	if err = json.Unmarshal(raw, record); err != nil {
		if scanerr.CodeOf(err) != "" {
			return nil, err
		}
		return nil, fmt.Errorf("malformed_input: %w", err)
	}
	return n, nil
}

// Every production destination is a fixed lock/manifest DTO. A decoded scalar
// or container reserves a 4KiB fixed DTO/index/library allowance, plus escaped strings,
// before the second, destination-specific standard JSON decode. This also
// covers geometric slice growth; no caller-supplied decoder callback is used.
func reserveRecord(st *budget.State, n *Node) error {
	rawBytes := int64(0)
	if n.Object == nil && n.Array == nil {
		rawBytes = 24 * int64(len(n.Raw))
	}
	if err := st.Working(4096 + rawBytes); err != nil {
		return err
	}
	for key, child := range n.Object {
		if err := st.Working(24 * int64(len(key))); err != nil {
			return err
		}
		if err := reserveRecord(st, child); err != nil {
			return err
		}
	}
	for _, child := range n.Array {
		if err := reserveRecord(st, child); err != nil {
			return err
		}
	}
	return nil
}

// Inspect a token boundary before Decoder.Token can allocate its string.
// JSON grammar validation remains owned by encoding/json.
func (p *reader) token() (json.Token, error) {
	start := int(p.dec.InputOffset())
	for start < len(p.raw) && bytes.IndexByte([]byte(" \t\r\n:,"), p.raw[start]) >= 0 {
		start++
	}
	if start < len(p.raw) && bytes.IndexByte([]byte("{}[]"), p.raw[start]) < 0 {
		end := start
		if p.raw[start] == '"' {
			end++
			for end < len(p.raw) {
				c := p.raw[end]
				end++
				if c == '"' {
					break
				}
				if c == '\\' && end < len(p.raw) {
					end++
				}
				if end-start > p.limits.MaxFieldBytes {
					return nil, scanerr.New(scanerr.ResourceLimit, "JSON scalar")
				}
			}
		} else {
			for end < len(p.raw) && bytes.IndexByte([]byte(" \t\r\n,]}"), p.raw[end]) < 0 {
				end++
			}
		}
		if end-start > p.limits.MaxFieldBytes {
			return nil, scanerr.New(scanerr.ResourceLimit, "JSON scalar")
		}
	}
	return p.dec.Token()
}
