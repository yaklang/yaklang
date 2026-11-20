package omap

import (
	"encoding/json"

	"github.com/yaklang/yaklang/common/log"
)

func Jsonify[T comparable, V any](v *OrderedMap[T, V]) []byte {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return raw
}

func (v *OrderedMap[T, V]) UnmarshalJSON(raw []byte) error {
	parsed := parseOrderedMapJSON(raw)
	switch parsed.kind {
	case 1:
		for _, e := range parsed.entries {
			var value V
			if err := json.Unmarshal([]byte(e.valRaw), &value); err != nil {
				log.Warnf("cannot unmarshal value: %v to %T", err, value)
				continue
			}
			v.Add(value)
		}
	case 2:
		for _, e := range parsed.entries {
			var val V
			if err := json.Unmarshal([]byte(e.valRaw), &val); err != nil {
				log.Warnf("cannot unmarshal value: %v to %T", err, val)
				continue
			}
			var keyResult T
			if err := json.Unmarshal([]byte(e.keyRaw), &keyResult); err != nil {
				continue
			}
			v.Set(keyResult, val)
		}
	case 3:
		v.literalValue = parsed.literal
	}
	return nil
}

func (v *OrderedMap[T, V]) MarshalJSONWithKeyValueFetcher(k func(t any) ([]byte, error), vf func(any) ([]byte, error)) ([]byte, error) {
	if v.haveLiteralValue() {
		return marshalOrderedMapJSON(true, v.literalValue, false, nil, nil, k, vf)
	}
	keys, vals := v.snapshotAny()
	return marshalOrderedMapJSON(false, nil, v.canAsList(), keys, vals, k, vf)
}

func (v *OrderedMap[T, V]) MarshalJSON() ([]byte, error) {
	return v.MarshalJSONWithKeyValueFetcher(nil, nil)
}
