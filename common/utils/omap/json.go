package omap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
)

func (v *OrderedMap[T, V]) Jsonify() []byte {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return raw
}

func (v *OrderedMap[T, V]) MarshalJSON() ([]byte, error) {
	if v.HaveLiteralValue() {
		raw, err := json.Marshal(v.LiteralValue())
		if err != nil {
			return nil, utils.Errorf("cannot marshal literal val: %T", v.LiteralValue())
		}
		return raw, nil
	}
	if v.CanAsList() {
		var buf bytes.Buffer
		buf.WriteByte('[')
		for index, val := range v.Values() {
			if index != 0 {
				buf.WriteByte(',')
			}
			raw, err := json.Marshal(val)
			if err != nil {
				buf.Write([]byte("null"))
				continue
			}
			buf.Write(raw)
		}
		buf.WriteByte(']')
		return buf.Bytes(), nil
	}

	var buf = new(bytes.Buffer)
	buf.WriteByte('{')
	first := true
	v.ForEach(func(i T, v V) bool {
		if first {
			first = false
		} else {
			buf.WriteByte(',')
		}
		fieldName := fmt.Sprint(i)
		_ = fieldName
		raw, _ := json.Marshal(fieldName)
		buf.Write(raw)
		buf.WriteByte(':')

		raw, err := json.Marshal(v)
		if err != nil {
			buf.Write([]byte("null"))
			return true
		}
		buf.Write(raw)
		return true
	})
	buf.WriteByte('}')

	return buf.Bytes(), nil
}
