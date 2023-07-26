package yaklib

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
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

func Escape(s []byte) string {
	var w strings.Builder
	xml.Escape(&w, s)
	return w.String()
}

func _xmldumps(v interface{}) []byte {
	var b bytes.Buffer

	v = StringMap(utils.InterfaceToGeneralMap(v))
	enc := xml.NewEncoder(&b)
	enc.Indent("  ", "  ")
	err := enc.Encode(v)
	if err != nil {
		panic(err)
	}
	return bytes.TrimSpace(b.Bytes())
}

func _xmlloads(v interface{}) StringMap {
	var buf bytes.Buffer
	i := make(StringMap)
	buf.Write([]byte("<root>"))
	buf.Write(utils.InterfaceToBytes(v))
	buf.Write([]byte("</root>"))
	xml.Unmarshal(buf.Bytes(), &i)
	return map[string]interface{}(i)
}

var XMLExports = map[string]interface{}{
	"Escape": Escape,
	"dumps":  _xmldumps,
	"loads":  _xmlloads,
}
