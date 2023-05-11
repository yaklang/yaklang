package utils

import (
	"encoding/json"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"reflect"
)

func MapGetStringOr(m map[string]interface{}, key string, value string) string {
	if m == nil {
		return value
	}

	r, ok := m[key]
	if ok {
		v, typeOk := r.(string)
		if typeOk {
			return v
		}
	}
	return value
}

func MapGetStringOr2(m map[string]string, key string, value string) string {
	if m == nil {
		return value
	}

	r, ok := m[key]
	if ok {
		return r
	}
	return value
}

func MapStringGetOr(m map[string]string, key string, value string) string {
	if m == nil {
		return value
	}

	r, ok := m[key]
	if ok {
		return r
	}

	return value
}

func MapStringGet(m map[string]string, key string) string {
	return MapStringGetOr(m, key, "")
}

func MapGetRaw(m map[string]interface{}, key string) interface{} {
	return MapGetRawOr(m, key, nil)
}

func MapGetFirstRaw(m map[string]interface{}, key ...string) interface{} {
	if len(key) <= 0 {
		return nil
	}

	for _, i := range key {
		result := MapGetRawOr(m, i, nil)
		if result == nil {
			continue
		}
		return result
	}
	return nil
}

func MapGetRawOr(m map[string]interface{}, key string, value interface{}) interface{} {
	if m == nil {
		return value
	}

	r, ok := m[key]
	if ok {
		return r
	} else {
		return value
	}
}

func MapGetString(m map[string]interface{}, key string) string {
	return MapGetStringOr(m, key, "")
}

func InterfaceToMapInterface(i interface{}) map[string]interface{} {
	raw, _ := InterfaceToMapInterfaceE(i)
	return raw
}

func InterfaceToMapInterfaceE(i interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{})
	if i == nil {
		return result, Error("empty")
	}
	switch ret := i.(type) {
	case map[string]interface{}:
		return ret, nil
	case map[string]string:
		for k, v := range ret {
			result[k] = v
		}
		return result, nil
	case map[interface{}]interface{}:
		result := make(map[string]interface{})
		for k, v := range ret {
			result[InterfaceToString(k)] = v
		}
		return result, nil
	default:
		if reflect.TypeOf(i).Kind() == reflect.Map {
			v := reflect.ValueOf(i)
			for _, k := range v.MapKeys() {
				result[InterfaceToString(k.Interface())] = v.MapIndex(k).Interface()
			}
			return result, nil
		} else {
			result["__[yaklang-raw]__"] = i
			log.Debugf("InterfaceToRawMap error: %v", i)
			return result, Errorf("interfaceToRawMap error, got: %v", spew.Sdump(i))
		}
	}
}

func MapGetString2(m map[string]string, key string) string {
	return MapGetStringOr2(m, key, "")
}

func MapGetMapRaw(m map[string]interface{}, key string) map[string]interface{} {
	return MapGetMapRawOr(m, key, make(map[string]interface{}))
}

func MapGetMapRawOr(m map[string]interface{}, key string, value map[string]interface{}) map[string]interface{} {
	if m == nil {
		return value
	}

	r, ok := m[key]
	if ok {
		data, typeOk := r.(map[string]interface{})
		if typeOk {
			return data
		}
	}
	return value
}

func MapGetIntOr(m map[string]interface{}, key string, value int) int {
	if m == nil {
		return value
	}

	r, ok := m[key]
	if ok {
		v, typeOk := r.(int)
		if typeOk {
			return v
		}
	}
	return value
}

func MapGetInt(m map[string]interface{}, key string) int {
	return MapGetIntOr(m, key, 0)
}

func MapGetFloat64Or(m map[string]interface{}, key string, value float64) float64 {
	if m == nil {
		return value
	}

	r, ok := m[key]
	if ok {
		v, typeOk := r.(float64)
		if typeOk {
			return v
		}
	}
	return value
}

func MapGetFloat64(m map[string]interface{}, key string) float64 {
	return MapGetFloat64Or(m, key, 0)
}

func MapGetFloat32Or(m map[string]interface{}, key string, value float32) float32 {
	if m == nil {
		return value
	}

	r, ok := m[key]
	if ok {
		v, typeOk := r.(float32)
		if typeOk {
			return v
		}
	}
	return value
}

func MapGetFloat32(m map[string]interface{}, key string) float32 {
	return MapGetFloat32Or(m, key, 0)
}

func MapGetBoolOr(m map[string]interface{}, key string, value bool) bool {
	if m == nil {
		return value
	}

	r, ok := m[key]
	if ok {
		v, typeOk := r.(bool)
		if typeOk {
			return v
		}
	}
	return value
}

func MapGetBool(m map[string]interface{}, key string) bool {
	return MapGetBoolOr(m, key, false)
}

func MapGetInt64Or(m map[string]interface{}, key string, value int64) int64 {
	if m == nil {
		return value
	}

	r, ok := m[key]
	if ok {
		v, typeOk := r.(int64)
		if typeOk {
			return v
		}
	}
	return value
}

func MapGetInt64(m map[string]interface{}, key string) int64 {
	return MapGetInt64Or(m, key, 0)
}

func InterfaceToGeneralMap(params interface{}) (finalResult map[string]interface{}) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("handle ptr/struct to map failed: %s", err)
			finalResult = map[string]interface{}{
				"__FALLBACK__": params,
			}
		}
	}()

	var p = map[string]interface{}{}
	setField := func(r reflect.Type, v reflect.Value, i int) {
		defer func() {
			if err := recover(); err != nil {
				key := r.Field(i)
				p[key.Name] = v.FieldByName(key.Name).Interface()
			}
		}()
	}
	pType := reflect.TypeOf(params)
	switch pType.Kind() {
	case reflect.Ptr:
		mapValue := reflect.ValueOf(params)
		res := mapValue.Elem()
		pType = reflect.TypeOf(res.Interface())
		for i := 0; i < res.NumField(); i++ {
			setField(pType, res, i)
		}
	case reflect.Struct:
		res := reflect.ValueOf(params)
		for i := 0; i < res.NumField(); i++ {
			setField(pType, res, i)
		}
	case reflect.Map:
		mapValue := reflect.ValueOf(params)
		for _, k := range mapValue.MapKeys() {
			valueRaw := mapValue.MapIndex(k)
			value := valueRaw.Interface()
			switch ret := value.(type) {
			case []byte:
				mapValue.SetMapIndex(k, reflect.ValueOf(string(ret)))
				p[k.String()] = string(ret)
			default:
				p[k.String()] = value
			}
		}
		return p
	default:
		p["__DEFAULT__"] = params
		return p
	}
	return p
}

func ToMapParams(params interface{}) (map[string]interface{}, error) {
	var p = map[string]interface{}{}
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, Errorf("marshal params failed: %s", err)
	}

	err = json.Unmarshal(raw, &p)
	if err != nil {
		return nil, Errorf("unmarshal map params failed: %s", err)
	}

	return p, nil
}

func MergeStringMap(ms ...map[string]string) map[string]string {
	res := map[string]string{}
	for _, m := range ms {
		for k, v := range m {
			res[k] = v
		}
	}
	return res
}
