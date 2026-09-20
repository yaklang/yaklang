// Package locktoml is the private finite Cargo/Poetry lock reader. The retained
// upstream lexer and structural parser do not include encoders, reflection
// decoders, formatting, file IO, environment options, or datetime support.
package locktoml

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"io"
	"strings"
	"unicode/utf8"
)

type parseAbort struct{ err error }
type Key []string

func (k Key) add(s string) Key { return append(append(Key{}, k...), s) }
func (k Key) String() string   { b, _ := json.Marshal([]string(k)); return string(b) }
func Parse(ctx context.Context, b []byte) (map[string]any, error) {
	p, e := document(ctx, b)
	if e != nil {
		return nil, e
	}
	return p.mapping, nil
}
func document(ctx context.Context, b []byte) (*parser, error) {
	if ctx == nil {
		return nil, fmt.Errorf("nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if int64(len(b)) > min(budget.From(ctx).Limits.MaxFileBytes, 16<<20) {
		return nil, fmt.Errorf("resource_limit: TOML bytes")
	}
	if !utf8.Valid(b) {
		return nil, fmt.Errorf("malformed_input: TOML requires UTF-8")
	}
	p, err := parse(ctx, string(b))
	if err != nil {
		return nil, err
	}
	return p, nil
}

type Span struct{ StartLine, EndLine int }

// DecodeRecords parses once, returning root array-table record spans directly
// from the token stream. Positions never depend on re-scanning quoted text.
func DecodeRecords(ctx context.Context, r io.Reader, out any, name string) ([]Span, error) {
	b, e := io.ReadAll(io.LimitReader(r, (16<<20)+1))
	if e != nil {
		return nil, e
	}
	p, e := document(ctx, b)
	if e != nil {
		return nil, e
	}
	raw, e := json.Marshal(p.mapping)
	if e != nil {
		return nil, e
	}
	if e = json.Unmarshal(raw, out); e != nil {
		return nil, e
	}
	var spans []Span
	if n := p.root.table[name]; n != nil && n.aot {
		for _, record := range n.array {
			spans = append(spans, Span{record.startLine, record.endLine})
		}
	}
	return spans, nil
}

// Decode converts only a bounded, validated map to fixed format-specific records.
func Decode(ctx context.Context, r io.Reader, out any) error {
	b, err := io.ReadAll(io.LimitReader(r, (16<<20)+1))
	if err != nil {
		return err
	}
	m, err := Parse(ctx, b)
	if err != nil {
		return err
	}
	b, err = json.Marshal(m)
	if err != nil {
		return fmt.Errorf("unsupported_syntax: TOML non-JSON scalar: %w", err)
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	return dec.Decode(out)
}
