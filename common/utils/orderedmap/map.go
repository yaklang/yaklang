package orderedmap

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
)

type OrderedMap struct {
	keys       []string
	values     map[string]any
	escapeHTML bool
}

func SetWithMaybeSameKey(o *OrderedMap, key string, value any) {
	oldValue, ok := o.values[key]
	if !ok {
		o.Set(key, value)
	} else {
		if oldValues, ok := oldValue.([]any); ok {
			oldValues = append(oldValues, value)
			o.values[key] = oldValues
		} else {
			o.values[key] = []any{oldValue, value}
		}
	}
}

// New 从零创建一个有序映射或从一个普通映射中创建一个有序映射，其的基本用法与普通映射相同，但内置方法可能不同
// Example:
// ```
// om = omap.New()
// om["a"] = 1
// om.b = 2
// println(om.a) // 1
// println(om["b"]) // 2
// om.Delete("a")
// om.Delete("b")
// println(om.a) // nil
// for i in 100 { om[string(i)] = i }
// for k, v in om {
// println(k, v)
// }
// ```
func New(maps ...any) *OrderedMap {
	m := make(map[string]any)
	if len(maps) > 0 {
		m = utils.InterfaceToMapInterface(maps[0])
	}

	o := OrderedMap{}
	o.keys = lo.Keys(m)
	o.values = m
	o.escapeHTML = true
	return &o
}

func (o *OrderedMap) SetEscapeHTML(on bool) {
	o.escapeHTML = on
}

func (o *OrderedMap) Get(key string) (any, bool) {
	val, exists := o.values[key]
	return val, exists
}

func (o *OrderedMap) Set(key string, value any) {
	_, exists := o.values[key]
	if !exists {
		o.keys = append(o.keys, key)
	}
	o.values[key] = value
}

func (o *OrderedMap) Merge(m *OrderedMap) {
	m.ForEach(func(key string, value any) {
		o.Set(key, value)
	})
}

func (o *OrderedMap) MergeStringMap(m map[string]string) {
	for k, v := range m {
		o.Set(k, v)
	}
}

func (o OrderedMap) String() string {
	return fmt.Sprintf("map[%s]", strings.Join(
		lo.Map(o.keys, func(k string, _ int) string {
			return fmt.Sprintf("%#v:%v", k, o.values[k])
		}), " "))
}

func (o OrderedMap) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v':
		io.WriteString(s, o.String())
	case 's':
		io.WriteString(s, o.String())
	case 'q':
		fmt.Fprintf(s, "%q", o.String())
	}
}

func (o *OrderedMap) Delete(key string) bool {
	// check key is in use
	_, ok := o.values[key]
	if !ok {
		return false
	}
	// remove from keys
	o.keys = lo.Filter(o.keys, func(k string, _ int) bool {
		return k != key
	})
	// remove from values
	delete(o.values, key)
	return true
}

func (o *OrderedMap) Keys() []string {
	return o.keys
}

func (o *OrderedMap) ToStringMap() map[string]any {
	return o.values
}

func (o *OrderedMap) Len() int {
	return len(o.keys)
}

func (o *OrderedMap) ForEach(fn func(key string, value any)) {
	for _, k := range o.keys {
		fn(k, o.values[k])
	}
}

func (o *OrderedMap) Range(fn func(key string, value any)) {
	o.ForEach(fn)
}

func (o *OrderedMap) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	*o = *New()

	var stack []*OrderedMap
	stack = append(stack, o)
	oldName := ""
	for {
		token, err := d.Token()
		if token == nil || err != nil {
			break
		}

		switch se := token.(type) {
		case xml.StartElement:
			name := se.Name.Local
			oldName = name
			node := New()

			top := stack[len(stack)-1]
			top.Set(name, node)
			stack = append(stack, node)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		case xml.CharData:
			val := strings.TrimSpace(string(se))
			if len(stack) >= 2 {
				top2 := stack[len(stack)-2]
				if val != "" {
					top2.Set(oldName, val)
				}
			}
		}
	}
	return nil
}

func (o *OrderedMap) UnmarshalJSON(b []byte) error {
	if o.values == nil {
		o.values = map[string]any{}
	}
	err := json.Unmarshal(b, &o.values)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	if _, err = dec.Token(); err != nil { // skip '{'
		return err
	}
	o.keys = make([]string, 0, len(o.values))
	return jsonDecodeOrderedMap(dec, o)
}

func xmlDecodeOrderedMap(dec *xml.Decoder, o *OrderedMap) error {
	hasKey := make(map[string]struct{}, len(o.values))
	for {
		token, err := dec.Token()
		if err != nil {
			return err
		}
		if _, ok := token.(xml.EndElement); ok {
			return nil
		}
		key := token.(string)
		if _, ok := hasKey[key]; ok {
			// duplicate key
			for j, k := range o.keys {
				if k == key {
					copy(o.keys[j:], o.keys[j+1:])
					break
				}
			}
			o.keys[len(o.keys)-1] = key
		} else {
			hasKey[key] = struct{}{}
			o.keys = append(o.keys, key)
		}

		token, err = dec.Token()
		if err != nil {
			return err
		}
		if _, ok := token.(xml.StartElement); ok {
			if values, ok := o.values[key].(map[string]any); ok {
				newMap := OrderedMap{
					keys:       make([]string, 0, len(values)),
					values:     values,
					escapeHTML: o.escapeHTML,
				}
				if err = xmlDecodeOrderedMap(dec, &newMap); err != nil {
					return err
				}
				o.values[key] = newMap
			} else if oldMap, ok := o.values[key].(OrderedMap); ok {
				newMap := OrderedMap{
					keys:       make([]string, 0, len(oldMap.values)),
					values:     oldMap.values,
					escapeHTML: o.escapeHTML,
				}
				if err = xmlDecodeOrderedMap(dec, &newMap); err != nil {
					return err
				}
				o.values[key] = newMap
			} else if err = xmlDecodeOrderedMap(dec, &OrderedMap{}); err != nil {
				return err
			}
		}
	}
}

func jsonDecodeOrderedMap(dec *json.Decoder, o *OrderedMap) error {
	hasKey := make(map[string]struct{}, len(o.values))
	for {
		token, err := dec.Token()
		if err != nil {
			return err
		}
		if delim, ok := token.(json.Delim); ok && delim == '}' {
			return nil
		}
		key := token.(string)
		if _, ok := hasKey[key]; ok {
			// duplicate key
			for j, k := range o.keys {
				if k == key {
					copy(o.keys[j:], o.keys[j+1:])
					break
				}
			}
			o.keys[len(o.keys)-1] = key
		} else {
			hasKey[key] = struct{}{}
			o.keys = append(o.keys, key)
		}

		token, err = dec.Token()
		if err != nil {
			return err
		}
		if delim, ok := token.(json.Delim); ok {
			switch delim {
			case '{':
				if values, ok := o.values[key].(map[string]any); ok {
					newMap := OrderedMap{
						keys:       make([]string, 0, len(values)),
						values:     values,
						escapeHTML: o.escapeHTML,
					}
					if err = jsonDecodeOrderedMap(dec, &newMap); err != nil {
						return err
					}
					o.values[key] = newMap
				} else if oldMap, ok := o.values[key].(OrderedMap); ok {
					newMap := OrderedMap{
						keys:       make([]string, 0, len(oldMap.values)),
						values:     oldMap.values,
						escapeHTML: o.escapeHTML,
					}
					if err = jsonDecodeOrderedMap(dec, &newMap); err != nil {
						return err
					}
					o.values[key] = newMap
				} else if err = jsonDecodeOrderedMap(dec, &OrderedMap{}); err != nil {
					return err
				}
			case '[':
				if values, ok := o.values[key].([]any); ok {
					if err = jsonDecodeSlice(dec, values, o.escapeHTML); err != nil {
						return err
					}
				} else if err = jsonDecodeSlice(dec, []any{}, o.escapeHTML); err != nil {
					return err
				}
			}
		}
	}
}

func jsonDecodeSlice(dec *json.Decoder, s []any, escapeHTML bool) error {
	for index := 0; ; index++ {
		token, err := dec.Token()
		if err != nil {
			return err
		}
		if delim, ok := token.(json.Delim); ok {
			switch delim {
			case '{':
				if index < len(s) {
					if values, ok := s[index].(map[string]any); ok {
						newMap := OrderedMap{
							keys:       make([]string, 0, len(values)),
							values:     values,
							escapeHTML: escapeHTML,
						}
						if err = jsonDecodeOrderedMap(dec, &newMap); err != nil {
							return err
						}
						s[index] = newMap
					} else if oldMap, ok := s[index].(OrderedMap); ok {
						newMap := OrderedMap{
							keys:       make([]string, 0, len(oldMap.values)),
							values:     oldMap.values,
							escapeHTML: escapeHTML,
						}
						if err = jsonDecodeOrderedMap(dec, &newMap); err != nil {
							return err
						}
						s[index] = newMap
					} else if err = jsonDecodeOrderedMap(dec, &OrderedMap{}); err != nil {
						return err
					}
				} else if err = jsonDecodeOrderedMap(dec, &OrderedMap{}); err != nil {
					return err
				}
			case '[':
				if index < len(s) {
					if values, ok := s[index].([]any); ok {
						if err = jsonDecodeSlice(dec, values, escapeHTML); err != nil {
							return err
						}
					} else if err = jsonDecodeSlice(dec, []any{}, escapeHTML); err != nil {
						return err
					}
				} else if err = jsonDecodeSlice(dec, []any{}, escapeHTML); err != nil {
					return err
				}
			case ']':
				return nil
			}
		}
	}
}

func (o OrderedMap) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	if len(o.keys) == 0 {
		return nil
	}
	if err := e.EncodeToken(start); err != nil {
		return err
	}
	for _, k := range o.keys {
		if err := e.EncodeElement(o.values[k], xml.StartElement{Name: xml.Name{Local: k}}); err != nil {
			return err
		}
	}
	return e.EncodeToken(start.End())
}

func (o OrderedMap) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(o.escapeHTML)
	for i, k := range o.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		// add key
		if err := encoder.Encode(k); err != nil {
			return nil, err
		}
		buf.WriteByte(':')
		// add value
		if err := encoder.Encode(o.values[k]); err != nil {
			return nil, err
		}
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}
