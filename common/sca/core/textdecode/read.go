// Package textdecode handles the frozen UTF-8/UTF-16 text snapshot encodings.
package textdecode

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/scanerr"
)

func Read(r io.Reader) ([]byte, error) {
	return ReadContext(context.Background(), r)
}

func ReadContext(ctx context.Context, r io.Reader) ([]byte, error) {
	ctx = budget.Ensure(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b, err := ReadRaw(ctx, r, min(budget.From(ctx).Limits.MaxFileBytes, 16<<20))
	if err != nil {
		return nil, err
	}
	return BOMContext(ctx, b)
}

// ReadRaw reserves each controlled backing allocation before reading into it.
// Replaced buffers remain charged, conservatively covering growth copy peaks.
// The extra byte detects oversized input without reading the rest of a stream.
func ReadRaw(ctx context.Context, r io.Reader, limit int64) ([]byte, error) {
	if ctx == nil || r == nil {
		return nil, scanerr.New(scanerr.InvalidInput, "nil text context or reader")
	}
	if limit < 0 || limit >= int64(int(^uint(0)>>1)) {
		return nil, scanerr.New(scanerr.InvalidConfig, "text byte limit")
	}
	st := budget.From(ctx)
	maxCapacity := int(limit) + 1
	var out []byte
	idle := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(out) == cap(out) {
			capacity := min(maxCapacity, 512)
			if cap(out) > 0 {
				if cap(out) > maxCapacity/2 {
					capacity = maxCapacity
				} else {
					capacity = cap(out) * 2
				}
			}
			// File limits are normally <=16MiB. Use checked arithmetic for
			// callers that explicitly select a larger bound.
			if capacity <= cap(out) {
				return nil, scanerr.New(scanerr.ResourceLimit, "text bytes")
			}
			if err := st.Working(budget.SizeOfBytes(capacity)); err != nil {
				return nil, err
			}
			next := make([]byte, len(out), capacity)
			copy(next, out)
			out = next
		}
		n, err := r.Read(out[len(out):cap(out)])
		if n < 0 || n > cap(out)-len(out) {
			return nil, scanerr.New(scanerr.InvalidInput, "invalid reader count")
		}
		out = out[:len(out)+n]
		if int64(len(out)) > limit {
			return nil, scanerr.New(scanerr.ResourceLimit, "text bytes")
		}
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		if n == 0 {
			idle++
			if idle >= 100 {
				return nil, io.ErrNoProgress
			}
		} else {
			idle = 0
		}
	}
}

// BOMContext reserves the UTF-16 output before BOM performs conversion.
// UTF-8 BOM removal is a slice operation and allocates no output copy.
func BOMContext(ctx context.Context, b []byte) ([]byte, error) {
	raw := bytes.TrimPrefix(b, []byte{0xef, 0xbb, 0xbf})
	if bytes.HasPrefix(raw, []byte{0xff, 0xfe}) || bytes.HasPrefix(raw, []byte{0xfe, 0xff}) {
		size, err := budget.SizeMul(len(b), 2)
		if err != nil {
			return nil, err
		}
		size, err = budget.SizeAdd(size, budget.SizeSlice)
		if err != nil {
			return nil, err
		}
		if err = budget.From(ctx).Working(size); err != nil {
			return nil, err
		}
	}
	return BOM(b)
}

func BOM(b []byte) ([]byte, error) {
	if bytes.HasPrefix(b, []byte{0xef, 0xbb, 0xbf}) {
		b = b[3:]
	}
	if bytes.HasPrefix(b, []byte{0xff, 0xfe}) {
		return decode16(b[2:], binary.LittleEndian)
	}
	if bytes.HasPrefix(b, []byte{0xfe, 0xff}) {
		return decode16(b[2:], binary.BigEndian)
	}
	if !utf8.Valid(b) {
		return nil, fmt.Errorf("unsupported_encoding: expected UTF-8 or UTF-16 BOM")
	}
	return b, nil
}
func decode16(b []byte, order binary.ByteOrder) ([]byte, error) {
	if len(b)%2 != 0 {
		return nil, fmt.Errorf("malformed_input: truncated UTF-16")
	}
	out := make([]byte, 0, len(b)/2*3)
	for i := 0; i < len(b); i += 2 {
		r := rune(order.Uint16(b[i:]))
		if utf16.IsSurrogate(r) {
			if r > 0xdbff || i+3 >= len(b) {
				return nil, fmt.Errorf("malformed_input: unpaired UTF-16 surrogate")
			}
			low := rune(order.Uint16(b[i+2:]))
			if low < 0xdc00 || low > 0xdfff {
				return nil, fmt.Errorf("malformed_input: unpaired UTF-16 surrogate")
			}
			r = utf16.DecodeRune(r, low)
			i += 2
		}
		out = utf8.AppendRune(out, r)
	}
	return out, nil
}

// CharsetReader is called by encoding/xml only for a non-UTF-8 declaration.
// Frozen POM fixtures use UTF-8. UTF-16, ASCII and ISO-8859-1 have small explicit
// conversions; arbitrary HTML encodings are deliberately not a codec framework.
func CharsetReader(label string, r io.Reader) (io.Reader, error) {
	return CharsetReaderContext(context.Background(), label, r)
}

func CharsetReaderContext(ctx context.Context, label string, r io.Reader) (io.Reader, error) {
	ctx = budget.Ensure(ctx)
	b, err := ReadRaw(ctx, r, min(budget.From(ctx).Limits.MaxFileBytes, 16<<20))
	if err != nil {
		return nil, err
	}
	if len(b) > 16<<20 {
		return nil, fmt.Errorf("resource_limit: XML encoding bytes")
	}
	encoding := strings.ToLower(strings.TrimSpace(label))
	switch encoding {
	case "utf-16", "utf-16le", "utf-16be", "iso-8859-1":
		size, e := budget.SizeMul(len(b), 2)
		if e != nil {
			return nil, e
		}
		size, e = budget.SizeAdd(size, budget.SizeSlice)
		if e != nil {
			return nil, e
		}
		if e = budget.From(ctx).Working(size); e != nil {
			return nil, e
		}
	}
	switch encoding {
	case "utf-8", "utf8":
		if !utf8.Valid(b) {
			return nil, fmt.Errorf("malformed_input: UTF-8")
		}
	case "utf-16":
		b, err = BOM(b)
	case "utf-16le":
		if bytes.HasPrefix(b, []byte{255, 254}) {
			b = b[2:]
		}
		b, err = decode16(b, binary.LittleEndian)
	case "utf-16be":
		if bytes.HasPrefix(b, []byte{254, 255}) {
			b = b[2:]
		}
		b, err = decode16(b, binary.BigEndian)
	case "us-ascii", "ascii":
		for _, c := range b {
			if c >= 128 {
				return nil, fmt.Errorf("malformed_input: ASCII")
			}
		}
	case "iso-8859-1":
		out := make([]byte, 0, len(b)*2)
		for _, c := range b {
			out = utf8.AppendRune(out, rune(c))
		}
		b = out
	default:
		return nil, fmt.Errorf("unsupported_encoding: %s", label)
	}
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(b), nil
}
