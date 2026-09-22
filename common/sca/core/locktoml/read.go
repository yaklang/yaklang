// Package locktoml is the private finite Cargo/Poetry lock reader. The retained
// upstream lexer and structural parser do not include encoders, reflection
// decoders, formatting, file IO, environment options, or datetime support.
package locktoml

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/textdecode"
	"io"
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
	if ctx != nil {
		ctx = budget.Ensure(ctx)
	}
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
	if err := budget.From(ctx).Working(int64(len(b))); err != nil {
		return nil, err
	}
	p, err := parse(ctx, string(b))
	if err != nil {
		return nil, err
	}
	return p, nil
}

type Span struct{ StartLine, EndLine int }

// ReadRecords returns the already parsed table and exact root record spans.
// Callers map their fixed Cargo/Poetry fields directly; there is no JSON
// serialization buffer or generic reflection destination.
func ReadRecords(ctx context.Context, r io.Reader, name string) (map[string]any, []Span, error) {
	if ctx != nil {
		ctx = budget.Ensure(ctx)
	}
	b, err := textdecode.ReadRaw(ctx, r, min(budget.From(ctx).Limits.MaxFileBytes, 16<<20))
	if err != nil {
		return nil, nil, err
	}
	p, err := document(ctx, b)
	if err != nil {
		return nil, nil, err
	}
	var spans []Span
	if n := p.root.table[name]; n != nil && n.aot {
		size, err := budget.SizeMul(len(n.array), 16)
		if err != nil {
			return nil, nil, err
		}
		if err = budget.From(ctx).Working(size + budget.SizeSlice); err != nil {
			return nil, nil, err
		}
		spans = make([]Span, len(n.array))
		for i, record := range n.array {
			spans[i] = Span{record.startLine, record.endLine}
		}
	}
	return p.mapping, spans, nil
}

// Field reads a fixed schema field without reflection or coercion. Missing
// fields keep their zero value, matching the lock DTO's prior JSON behavior.
func Field[T any](m map[string]any, key string) (T, error) {
	var zero T
	v, ok := m[key]
	if !ok {
		return zero, nil
	}
	value, ok := v.(T)
	if !ok {
		return zero, fmt.Errorf("malformed_input: TOML field %q has incompatible type", key)
	}
	return value, nil
}
func Table(v any) (map[string]any, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("malformed_input: TOML record must be a table")
	}
	return m, nil
}
