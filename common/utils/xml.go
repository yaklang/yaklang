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

type StringMap map[string]interface{}

func (m StringMap) marshalXML(e *xml.Encoder, start xml.StartElement, first bool) error {
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
			err = e.EncodeElement(v, childStart)
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
	return m.marshalXML(e, start, true)
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

func XmlDumps(v interface{}) []byte {
	var b bytes.Buffer

	v = StringMap(InterfaceToGeneralMap(v))
	enc := xml.NewEncoder(&b)
	enc.Indent("", "  ")
	err := enc.Encode(v)
	if err != nil {
		panic(err)
	}
	return b.Bytes()
}

func XmlLoads(v interface{}) map[string]any {
	var buf bytes.Buffer
	i := make(StringMap)
	buf.Write([]byte("<root>"))
	buf.Write(InterfaceToBytes(v))
	buf.Write([]byte("</root>"))
	decoder := xml.NewDecoder(&buf)
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
