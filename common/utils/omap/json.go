package omap

import (
	"bytes"
	"encoding/json"

	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func (v *OrderedMap[T, V]) Jsonify() []byte {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return raw
}

func (v *OrderedMap[T, V]) UnmarshalJSON(raw []byte) error {
	gresult := gjson.ParseBytes(raw)

	// as list
	if gresult.IsArray() {
		for _, r := range gresult.Array() {
			var value V
			err := json.Unmarshal([]byte(r.Raw), &value)
			if err != nil {
				// [WARN] 2024-04-22 00:38:35 [json:29] cannot unmarshal value: json: cannot unmarshal object into Go value of type ssautil.VersionedIF[github.com/yaklang/yaklang/common/yak/ssa.Value] to <nil>
				log.Warnf("cannot unmarshal value: %v to %T", err, value)
				continue
			}
			v.Push(value)
		}
		return nil
	} else if gresult.IsObject() {
		gresult.ForEach(func(key, value gjson.Result) bool {
			var val V
			err := json.Unmarshal([]byte(value.Raw), &val)
			if err != nil {
				log.Warnf("cannot unmarshal value: %v to %T", err, val)
				return true
			}
			var keyResult T
			err = json.Unmarshal([]byte(key.Raw), &keyResult)
			if err != nil {
				return true
			}
			v.Set(keyResult, val)
			return true
		})
		return nil
	} else if gresult.IsBool() {
		v.SetLiteralValue(gresult.Bool())
		return nil
	} else {
		v.SetLiteralValue(gresult.Num)
	}
	return nil
}

func (v *OrderedMap[T, V]) MarshalJSONWithKeyValueFetcher(k func(t any) ([]byte, error), vf func(any) ([]byte, error)) ([]byte, error) {
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
			var raw []byte
			var err error
			if vf != nil {
				raw, err = vf(val)
			} else {
				var vAny any = val
				if vIns, ok := vAny.(interface {
					MarshalJSONWithKeyValueFetcher(k func(t any) ([]byte, error), vf func(any) ([]byte, error)) ([]byte, error)
				}); ok {
					raw, err = vIns.MarshalJSONWithKeyValueFetcher(k, vf)
				} else {
					raw, err = json.Marshal(val)
				}
			}
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

		var raw []byte
		var err error
		if k != nil {
			raw, err = k(i)
		} else {
			raw, err = json.Marshal(i)
		}
		buf.Write(raw)
		buf.WriteByte(':')

		if vf != nil {
			raw, err = vf(v)
		} else {
			var vAny any = v
			if vIns, ok := vAny.(interface {
				MarshalJSONWithKeyValueFetcher(k func(t any) ([]byte, error), vf func(any) ([]byte, error)) ([]byte, error)
			}); ok {
				raw, err = vIns.MarshalJSONWithKeyValueFetcher(k, vf)
			} else {
				raw, err = json.Marshal(v)
			}
		}
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

func (v *OrderedMap[T, V]) MarshalJSON() ([]byte, error) {
	return v.MarshalJSONWithKeyValueFetcher(nil, nil)
}
