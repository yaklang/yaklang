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
)

func Read(r io.Reader) ([]byte, error) {
	return ReadContext(context.Background(), r)
}

func ReadContext(ctx context.Context, r io.Reader) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	capn := min(budget.From(ctx).Limits.MaxFileBytes, 16<<20)
	b, err := io.ReadAll(io.LimitReader(r, capn+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > capn {
		return nil, fmt.Errorf("resource_limit: text bytes")
	}
	if err := budget.From(ctx).Working(int64(len(b))); err != nil {
		return nil, err
	}
	out, err := BOM(b)
	if err != nil {
		return nil, err
	}
	if len(out) != len(b) {
		if err := budget.From(ctx).Working(int64(len(out))); err != nil {
			return nil, err
		}
	}
	return out, nil
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
	out := make([]byte, 0, len(b))
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
	b, err := io.ReadAll(io.LimitReader(r, (16<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(b) > 16<<20 {
		return nil, fmt.Errorf("resource_limit: XML encoding bytes")
	}
	switch strings.ToLower(strings.TrimSpace(label)) {
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
