// Package xmlrecord bounds standard XML tokenization before fixed-record decode.
package xmlrecord

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/scanerr"
	"github.com/yaklang/yaklang/common/sca/core/textdecode"
	"io"
	"strings"
)

type tokens struct {
	ctx          context.Context
	d            *xml.Decoder
	depth, count int
	l            budget.Limits
}

func (t *tokens) Token() (xml.Token, error) {
	if err := t.ctx.Err(); err != nil {
		return nil, err
	}
	if err := budget.From(t.ctx).Add(1, budget.SizeObject); err != nil {
		return nil, err
	}
	token, err := t.d.Token()
	if err != nil {
		return nil, err
	}
	switch v := token.(type) {
	case xml.StartElement:
		t.depth++
		if t.depth > t.l.MaxSyntaxDepth {
			return nil, fmt.Errorf("resource_limit: XML depth")
		}
		if len(v.Attr) > 10000 {
			return nil, fmt.Errorf("resource_limit: XML attributes")
		}
		for _, a := range v.Attr {
			if len(a.Value) > t.l.MaxFieldBytes {
				return nil, fmt.Errorf("resource_limit: XML attribute")
			}
			if err := budget.From(t.ctx).Result(budget.SizeOfString(a.Value)); err != nil {
				return nil, err
			}
		}
	case xml.EndElement:
		t.depth--
	case xml.CharData:
		if len(v) > t.l.MaxFieldBytes {
			return nil, fmt.Errorf("resource_limit: XML text")
		}
		if err := budget.From(t.ctx).Result(budget.SizeOfBytes(len(v))); err != nil {
			return nil, err
		}
	case xml.Directive:
		return nil, fmt.Errorf("unsupported_syntax: XML directives")
	}
	return token, nil
}

// Record is implemented by the fixed POM DTO. No arbitrary type inspection or
// registration is performed; schema changes require a corresponding plan/test.
type Record interface{ XMLRecordKind() Kind }
type Kind uint8

const POM Kind = 1

func Decode(ctx context.Context, r io.Reader, out Record) error {
	if out == nil || out.XMLRecordKind() != POM {
		return fmt.Errorf("unsupported_syntax: XML record schema")
	}
	return decodeWithPlan(ctx, r, out, &pomLayout)
}

func decodeWithPlan(ctx context.Context, r io.Reader, out any, plan *destPlan) error {
	l := budget.From(ctx).Limits
	capn := min(l.MaxFileBytes, 16<<20)
	if l.MaxResultBytes > 0 && l.MaxResultBytes < capn {
		capn = l.MaxResultBytes
	}
	b, err := textdecode.ReadRaw(ctx, r, capn)
	if err != nil {
		return err
	}
	if err := budget.From(ctx).Result(budget.SizeDecoderScratch); err != nil {
		return err
	}
	transcoded := bytes.HasPrefix(b, []byte{0xff, 0xfe}) || bytes.HasPrefix(b, []byte{0xfe, 0xff})
	if transcoded || bytes.HasPrefix(b, []byte{0xef, 0xbb, 0xbf}) {
		b, err = textdecode.BOMContext(ctx, b)
		if err != nil {
			return err
		}
	}
	hits, props, err := countFixedStarts(ctx, decoderWithCharsetContext(ctx, b, transcoded), l, plan)
	if err != nil {
		return err
	}
	if err = chargeDestination(ctx, plan, hits, props); err != nil {
		return err
	}
	if err = budget.From(ctx).Working(int64(len(b))); err != nil {
		return err
	}
	decoder := decoderWithCharsetContext(ctx, b, transcoded)
	if err = decoder.Decode(out); err != nil {
		if scanerr.CodeOf(err) == scanerr.ResourceLimit || scanerr.CodeOf(err) == scanerr.UnsupportedSyntax {
			return err
		}
		return fmt.Errorf("malformed_input: XML: %w", err)
	}
	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch v := tok.(type) {
		case xml.CharData:
			if strings.TrimSpace(string(v)) != "" {
				return fmt.Errorf("malformed_input: XML trailing content")
			}
		case xml.Comment:
		default:
			return fmt.Errorf("malformed_input: XML trailing element")
		}
	}
}

func decoderWithCharset(b []byte, transcoded bool) *xml.Decoder {
	return decoderWithCharsetContext(context.Background(), b, transcoded)
}
func decoderWithCharsetContext(ctx context.Context, b []byte, transcoded bool) *xml.Decoder {
	d := xml.NewDecoder(bytes.NewReader(b))
	d.CharsetReader = func(charset string, input io.Reader) (io.Reader, error) {
		if transcoded {
			switch strings.ToLower(charset) {
			case "utf-16", "utf-16le", "utf-16be":
				return input, nil
			default:
				return nil, fmt.Errorf("malformed_input: encoding declaration conflicts with UTF-16 BOM")
			}
		}
		return textdecode.CharsetReaderContext(ctx, charset, input)
	}
	return d
}
