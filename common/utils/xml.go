package utils

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"golang.org/x/net/html/charset"
)

// XMLEncoder 扩展标准的 xml.Encoder
type XMLEncoder struct {
	*xml.Encoder
	escapeHTML bool
}

// EncoderOption 定义编码器选项的函数类型
type EncoderOption func(*XMLEncoder)

// NewXMLEncoder 创建新的 XMLEncoder
func NewXMLEncoder(w io.Writer, options ...EncoderOption) *XMLEncoder {
	e := &XMLEncoder{
		Encoder:    xml.NewEncoder(w),
		escapeHTML: true, // 默认转义 HTML
	}

	// 应用选项
	for _, opt := range options {
		opt(e)
	}

	return e
}

// WithHTMLEscape 设置是否转义 HTML 的选项，默认为 True，即转义 HTML
// Example:
// ```
// m = {"a": "qwe&zxc"}
// e := xml.dumps(m, xml.escape(false))
// ```
func WithHTMLEscape(escape bool) EncoderOption {
	return func(e *XMLEncoder) {
		e.escapeHTML = escape
	}
}

// 用于存储原始 HTML 内容
type RawString string

func (h RawString) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	// 直接将 HTML 内容作为原始标记写入
	tokens := []xml.Token{
		start,
		xml.CharData(h),
		start.End(),
	}

	for _, t := range tokens {
		err := e.EncodeToken(t)
		if err != nil {
			return err
		}
	}
	return e.Flush()
}

type StringMap map[string]interface{}

func (m StringMap) marshalXML(e *XMLEncoder, start xml.StartElement, first bool) error {
	var err error
	if !first {
		err = e.EncodeToken(start)
		if err != nil {
			return err
		}
	}
	for key, val := range m {
		childStart := xml.StartElement{Name: xml.Name{Local: key}}
		switch v := val.(type) {
		case string:
			if !e.escapeHTML {
				err = e.EncodeElement(RawString(v), childStart)
			} else {
				err = e.EncodeElement(v, childStart)
			}
		case StringMap:
			err = v.marshalXML(e, childStart, false)
		case map[string]interface{}:
			err = StringMap(v).marshalXML(e, childStart, false)
		default:
			return fmt.Errorf("unsupported type: %T", v)
		}
		if err != nil {
			return err
		}
	}
	if !first {
		return e.EncodeToken(start.End())
	}
	return nil
}

func (m StringMap) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	return m.marshalXML(&XMLEncoder{
		Encoder:    e,
		escapeHTML: true, // 使用默认设置
	}, start, true)
}

func (m *StringMap) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	*m = StringMap{}

	var stack []StringMap
	stack = append(stack, *m)
	oldName := ""
	_ = oldName
	for {
		token, err := d.Token()
		if token == nil || err != nil {
			break
		}

		switch se := token.(type) {
		case xml.StartElement:
			name := se.Name.Local
			oldName = name
			node := make(map[string]interface{})

			top := stack[len(stack)-1]
			if _, ok := top[name]; ok {
				// 处理相同标签名的情况
				switch v := top[name].(type) {
				case []interface{}:
					top[name] = append(v, node)
				default:
					top[name] = []interface{}{v, node}
				}
				stack = append(stack, node)
			} else {
				top[name] = node
				stack = append(stack, node)
			}

		case xml.EndElement:
			stack = stack[:len(stack)-1]
		case xml.CharData:
			val := strings.TrimSpace(string(se))
			if len(stack) >= 2 {
				top2 := stack[len(stack)-2]
				if val != "" {
					top2[oldName] = val
				}
			}
		}
	}

	return nil
}

func XmlEscape(s []byte) string {
	var w strings.Builder
	xml.Escape(&w, s)
	return w.String()
}

func XmlDumps(v interface{}, opts ...EncoderOption) []byte {
	var b bytes.Buffer

	v = StringMap(InterfaceToGeneralMap(v))
	enc := NewXMLEncoder(&b, opts...)
	enc.Indent("", "  ")
	err := enc.Encode(v)
	if err != nil {
		log.Errorf("xml encode error: %v", err)
	}
	return b.Bytes()
}

func XmlLoads(v interface{}) map[string]any {
	i := make(StringMap)
	buf := bytes.NewBufferString(fmt.Sprintf("<root>%s</root>", InterfaceToString(v)))
	decoder := xml.NewDecoder(buf)
	decoder.CharsetReader = func(label string, input io.Reader) (io.Reader, error) {
		e, _ := charset.Lookup(label)
		if e != nil {
			return e.NewDecoder().Reader(input), nil
		}
		return input, nil // default to utf-8
	}
	err := decoder.Decode(&i)
	if err != nil {
		log.Errorf("xml decode error: %v", err)
	}
	return map[string]any(i)
}
