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
	if err := budget.From(ctx).Working(int64(len(raw))); err != nil {
		return nil, err
	}
	if err := budget.From(ctx).Result(budget.SizeDecoderScratch); err != nil {
		return nil, err
	}
	newlines := bytes.Count(raw, []byte{'\n'})
	if err := budget.From(ctx).Result(budget.SizeSlice + int64(newlines)*8); err != nil {
		return nil, err
	}
	p := &reader{limits: l, ctx: ctx, raw: raw, dec: json.NewDecoder(bytes.NewReader(raw))}
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
	if _, err = p.dec.Token(); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("trailing JSON data")
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
	tok, err := p.dec.Token()
	if err != nil {
		return nil, fmt.Errorf("malformed_input: %w", err)
	}
	n := &Node{}
	if d, ok := tok.(json.Delim); ok {
		switch d {
		case '{':
			n.Object = map[string]*Node{}
			for p.dec.More() {
				key, err := p.dec.Token()
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
				if err := budget.From(p.ctx).Result(budget.SizePtr); err != nil {
					return nil, err
				}
				n.Array = append(n.Array, v)
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
		return nil, fmt.Errorf("resource_limit: JSON scalar")
	}
	n.Raw = p.raw[start:end]
	n.StartLine = sort.SearchInts(p.lines, start) + 1
	n.EndLine = sort.SearchInts(p.lines, end-1) + 1
	return n, nil
}

// Decode validates the complete document, then decodes only into the caller's
// fixed record type. The returned nodes are used explicitly for field locations.
func Decode(ctx context.Context, raw []byte, record any) (*Node, error) {
	n, err := Parse(ctx, raw)
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(raw, record); err != nil {
		return nil, fmt.Errorf("malformed_input: %w", err)
	}
	return n, nil
}
