package orderedmap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/yaklang/yaklang/common/utils/yakxml"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
)

type OrderedMap struct {
	*OrderedMapEx[string, any]
}

type OrderedMapEx[K comparable, V any] struct {
	keys       []K
	values     map[K]V
	escapeHTML bool
}

func NewOrderMapEx[K comparable, V any](key []K, value map[K]V, escapeHTML bool) *OrderedMapEx[K, V] {
	if value == nil {
		value = make(map[K]V)
	}
	if key == nil {
		key = make([]K, 0, len(value))
	}
	return &OrderedMapEx[K, V]{
		keys:       key,
		values:     value,
		escapeHTML: escapeHTML,
	}
}

func NewOrderMap(key []string, value map[string]any, escapeHTML bool) *OrderedMap {
	return &OrderedMap{
		OrderedMapEx: NewOrderMapEx(key, value, escapeHTML),
	}
}

func SetAny(o *OrderedMap, key any, value any) {
	o.Set(utils.InterfaceToString(key), value)
}

// New 从零创建一个有序映射或从一个普通映射中创建一个有序映射，其的基本用法与普通映射相同，但内置方法可能不同
// 值得注意的是，如果传入的是一个普通映射，使用此函数转换为有序映射并不能保证原有的顺序
// 如果需要保留原有顺序，可以使用 `omap({"a": 1, "b": 2})` 来直接生成一个有序映射
// Example:
// ```
// om = orderedmap.New()
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
	var m map[string]any
	if len(maps) > 0 {
		if om, ok := maps[0].(*OrderedMap); ok {
			return om
		}
		m = utils.InterfaceToMapInterface(maps[0])
	} else {
		m = make(map[string]any)
	}

	for k, v := range m {
		if utils.IsNil(v) {
			continue
		}
		if reflect.TypeOf(v).Kind() == reflect.Map {
			m[k] = New(v)
		}
	}

	return NewOrderMap(lo.Keys(m), m, false)
}

func (o *OrderedMapEx[K, V]) Copy() *OrderedMapEx[K, V] {
	m := make(map[K]V, len(o.values))
	for k, v := range o.values {
		if utils.IsNil(v) {
			continue
		}
		m[k] = v
	}
	return NewOrderMapEx(o.keys, m, o.escapeHTML)
}

func (o *OrderedMap) Copy() *OrderedMap {
	m := utils.InterfaceToMapInterface(o.values)
	for k, v := range m {
		if utils.IsNil(v) {
			continue
		}
		if reflect.TypeOf(v).Kind() == reflect.Map {
			m[k] = New(v)
		}
	}

	return NewOrderMap(o.keys, m, o.escapeHTML)
}

func (o *OrderedMap) SetEscapeHTML(on bool) {
	o.escapeHTML = on
}

func (o *OrderedMapEx[K, V]) Get(key K) (V, bool) {
	val, exists := o.values[key]
	return val, exists
}

func (o *OrderedMapEx[K, V]) Set(key K, value V) {
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
	return fmt.Sprintf("map[%v]", strings.Join(
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

func (o *OrderedMapEx[K, V]) Keys() []K {
	return o.keys
}

func (o *OrderedMapEx[K, V]) Values() []V {
	values := make([]V, len(o.keys))
	for i, k := range o.keys {
		values[i] = o.values[k]
	}
	return values
}

func (o *OrderedMap) ToStringMap() map[string]any {
	stringMap := make(map[string]any, len(o.values))
	for _, k := range o.keys {
		switch ret := o.values[k].(type) {
		case *OrderedMap:
			stringMap[k] = ret.ToStringMap()
		case OrderedMap:
			stringMap[k] = ret.ToStringMap()
		default:
			stringMap[k] = ret
		}
	}
	return stringMap
}

func (o *OrderedMap) ToAnyMap() map[any]any {
	return lo.MapEntries(o.values, func(key string, value any) (any, any) {
		return key, value
	})
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

func (o *OrderedMap) UnmarshalXML(d *yakxml.Decoder, start yakxml.StartElement) error {
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
		case yakxml.StartElement:
			name := se.Name.Local
			oldName = name
			node := New()

			top := stack[len(stack)-1]
			top.Set(name, node)
			stack = append(stack, node)
		case yakxml.EndElement:
			stack = stack[:len(stack)-1]
		case yakxml.CharData:
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

func xmlDecodeOrderedMap(dec *yakxml.Decoder, o *OrderedMap) error {
	hasKey := make(map[string]struct{}, len(o.values))
	for {
		token, err := dec.Token()
		if err != nil {
			return err
		}
		if _, ok := token.(yakxml.EndElement); ok {
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
		if _, ok := token.(yakxml.StartElement); ok {
			if values, ok := o.values[key].(map[string]any); ok {
				keys := make([]string, 0, len(values))
				newMap := NewOrderMap(keys, values, o.escapeHTML)
				// newMap := OrderedMap{
				// 	keys:       make([]string, 0, len(values)),
				// 	values:     values,
				// 	escapeHTML: o.escapeHTML,
				// }
				if err = xmlDecodeOrderedMap(dec, newMap); err != nil {
					return err
				}
				o.values[key] = newMap
			} else if oldMap, ok := o.values[key].(OrderedMap); ok {
				keys := make([]string, 0, len(oldMap.values))
				newMap := NewOrderMap(keys, oldMap.values, oldMap.escapeHTML)
				if err = xmlDecodeOrderedMap(dec, newMap); err != nil {
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
					keys := make([]string, 0, len(values))
					newMap := NewOrderMap(keys, values, o.escapeHTML)
					if err = jsonDecodeOrderedMap(dec, newMap); err != nil {
						return err
					}
					o.values[key] = newMap
				} else if oldMap, ok := o.values[key].(OrderedMap); ok {
					keys := make([]string, 0, len(oldMap.values))
					newMap := NewOrderMap(keys, oldMap.values, oldMap.escapeHTML)
					if err = jsonDecodeOrderedMap(dec, newMap); err != nil {
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
						keys := make([]string, 0, len(values))
						newMap := NewOrderMap(keys, values, escapeHTML)
						if err = jsonDecodeOrderedMap(dec, newMap); err != nil {
							return err
						}
						s[index] = newMap
					} else if oldMap, ok := s[index].(OrderedMap); ok {
						keys := make([]string, 0, len(oldMap.values))
						newMap := NewOrderMap(keys, oldMap.values, escapeHTML)
						if err = jsonDecodeOrderedMap(dec, newMap); err != nil {
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

func (o OrderedMap) marshalXML(e *yakxml.Encoder, start yakxml.StartElement, first bool) error {
	var err error
	if !first {
		err = e.EncodeToken(start)
		if err != nil {
			return err
		}
	}

	for _, k := range o.keys {
		v := o.values[k]
		childStart := yakxml.StartElement{Name: yakxml.Name{Local: k}}

		switch ret := v.(type) {
		case OrderedMap:
			if err := ret.marshalXML(e, childStart, false); err != nil {
				return err
			}
		case *OrderedMap:
			if err := ret.marshalXML(e, childStart, false); err != nil {
				return err
			}
		default:
			if err := e.EncodeElement(v, childStart); err != nil {
				return err
			}
		}
	}
	if !first {
		return e.EncodeToken(start.End())
	}
	return nil
}

func (m OrderedMap) MarshalXML(e *yakxml.Encoder, start yakxml.StartElement) error {
	return m.marshalXML(e, start, true)
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
