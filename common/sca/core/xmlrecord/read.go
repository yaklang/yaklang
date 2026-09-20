// Package xmlrecord bounds standard XML tokenization before fixed-record decode.
package xmlrecord

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"github.com/yaklang/yaklang/common/sca/core/budget"
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
	token, err := t.d.Token()
	if err != nil {
		return nil, err
	}
	if err := budget.From(t.ctx).Add(1, budget.SizeObject); err != nil {
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
		}
	case xml.EndElement:
		t.depth--
	case xml.CharData:
		if len(v) > t.l.MaxFieldBytes {
			return nil, fmt.Errorf("resource_limit: XML text")
		}
	case xml.Directive:
		return nil, fmt.Errorf("unsupported_syntax: XML directives")
	}
	return token, nil
}
func Decode(ctx context.Context, r io.Reader, out any) error {
	l := budget.From(ctx).Limits
	b, err := io.ReadAll(io.LimitReader(r, min(l.MaxFileBytes, 16<<20)+1))
	if err != nil {
		return err
	}
	if int64(len(b)) > min(l.MaxFileBytes, 16<<20) {
		return fmt.Errorf("resource_limit: XML bytes")
	}
	if err := budget.From(ctx).Working(int64(len(b))); err != nil {
		return err
	}
	transcoded := bytes.HasPrefix(b, []byte{0xff, 0xfe}) || bytes.HasPrefix(b, []byte{0xfe, 0xff})
	if transcoded || bytes.HasPrefix(b, []byte{0xef, 0xbb, 0xbf}) {
		b, err = textdecode.BOM(b)
		if err != nil {
			return err
		}
		if err := budget.From(ctx).Working(int64(len(b))); err != nil {
			return err
		}
	}
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
		return textdecode.CharsetReader(charset, input)
	}
	guard := &tokens{ctx: ctx, d: d, l: l}
	decoder := xml.NewTokenDecoder(guard)
	if err = decoder.Decode(out); err != nil {
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
