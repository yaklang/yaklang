package utils

import (
	"encoding/json"
	"fmt"
	"golang.org/x/exp/maps"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func MarshalIdempotent(v interface{}) ([]byte, error) {
	var order func(v any) any
	order = func(v any) any {
		if v == nil {
			return nil
		}
		refV := reflect.ValueOf(v)
		if refV.Kind() == reflect.Ptr {
			refV = refV.Elem()
		}
		switch refV.Kind() {
		case reflect.Map:
			res := [][2]any{}
			keys := refV.MapKeys()
			sort.Slice(keys, func(i, j int) bool {
				return InterfaceToString(keys[i].Interface()) < InterfaceToString(keys[j].Interface())
			})
			for _, k := range keys {
				res = append(res, [2]any{k.Interface(), order(refV.MapIndex(k).Interface())})
			}
			return res
		case reflect.Slice, reflect.Array:
			res := []any{}
			for i := 0; i < refV.Len(); i++ {
				res = append(res, order(refV.Index(i).Interface()))
			}
			return res
		}
		return v
	}
	// 执行JSON编码
	return json.Marshal(order(v))
}

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
		if result != nil {
			return result
		}

		// If not, try to find the key with "request_%d" format
		for j := 1; j <= 20; j++ {
			reqKey := fmt.Sprintf("%s_%d", i, j)
			result := MapGetRawOr(m, reqKey, nil)
			if result != nil {
				return result
			}
		}
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

func MapGetStringSlice(m map[string]interface{}, key string) []string {
	return InterfaceToStringSlice(MapGetRaw(m, key))
}

func MapGetStringByManyFields(m map[string]interface{}, key ...string) string {
	if len(key) <= 0 {
		return ""
	}

	for _, i := range key {
		result := MapGetStringOr(m, i, "")
		if result != "" {
			return result
		}
	}
	return ""
}

func ExtractMapValueString(m any, key string) string {
	return MapGetString(ParseStringToGeneralMap(m), key)
}

func ExtractMapValueInt(m any, key string) int {
	return MapGetInt(ParseStringToGeneralMap(m), key)
}

func ExtractMapValueBool(m any, key string) bool {
	return MapGetBool(ParseStringToGeneralMap(m), key)
}

func ExtractMapValueGeneralMap(m any, key string) map[string]any {
	return MapGetMapRaw(ParseStringToGeneralMap(m), key)
}

func ExtractMapValueRaw(m any, key string) any {
	return MapGetRaw(ParseStringToGeneralMap(m), key)
}

func InterfaceToMapInterface(i interface{}) map[string]interface{} {
	raw, _ := InterfaceToMapInterfaceE(i)
	return raw
}

func InterfaceToSliceInterface(i interface{}) []any {
	raw, _ := InterfaceToSliceInterfaceE(i)
	return raw
}

func InterfaceToSliceInterfaceE(i interface{}) ([]any, error) {
	result := make([]any, 0)
	if i == nil {
		return result, Error("empty")
	}
	switch ret := i.(type) {
	case []interface{}:
		for _, v := range ret {
			result = append(result, v)
		}
		return result, nil
	default:
		if reflect.TypeOf(i).Kind() == reflect.Slice {
			v := reflect.ValueOf(i)
			for j := 0; j < v.Len(); j++ {
				result = append(result, v.Index(j).Interface())
			}
			return result, nil
		} else {
			result = append(result, i)
			return result, Errorf("interfaceToRawMap error, got: %v", spew.Sdump(i))
		}
	}
}

func InterfaceToMapInterfaceE(i interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{})
	if i == nil {
		return result, Error("empty")
	}
	switch ret := i.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{})
		maps.Copy(result, ret)
		return result, nil
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
		return InterfaceToGeneralMap(r)
	}
	return value
}

func MapGetIntOr(m map[string]interface{}, key string, value int) int {
	if m == nil {
		return value
	}

	r, ok := m[key]
	if ok {
		result, err := strconv.ParseInt(fmt.Sprint(r), 10, 64)
		if err != nil {
			return value
		}
		return int(result)
	}
	return value
}

func MapGetInt(m map[string]interface{}, key string) int {
	return MapGetIntOr(m, key, 0)
}

func MapGetIntEx(m map[string]interface{}, key ...string) int {
	return codec.Atoi(InterfaceToString(MapGetFirstRaw(m, key...)))
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
		return int64(InterfaceToInt(r))
	}
	return value
}

func MapGetInt64(m map[string]interface{}, key string) int64 {
	return MapGetInt64Or(m, key, 0)
}

func MapGetMapStringInt64(m map[string]interface{}, key string) map[string]int64 {
	if m == nil {
		return map[string]int64{}
	}

	r, ok := m[key]
	if !ok {
		return map[string]int64{}
	}

	switch v := r.(type) {
	case map[string]int64:
		return v
	case map[string]interface{}:
		result := make(map[string]int64, len(v))
		for k, item := range v {
			result[k] = int64(InterfaceToInt(item))
		}
		return result
	default:
		return map[string]int64{}
	}
}

func MapGetInt64Slice(m map[string]interface{}, key string) []int64 {
	if m == nil {
		return []int64{}
	}

	r, ok := m[key]
	if !ok {
		return []int64{}
	}

	switch v := r.(type) {
	case []int64:
		return v
	case []interface{}:
		result := make([]int64, 0, len(v))
		for _, item := range v {
			result = append(result, int64(InterfaceToInt(item)))
		}
		return result
	default:
		typ := reflect.TypeOf(r)
		if typ == nil {
			return nil
		}
		t := typ.Kind()
		len := reflect.ValueOf(r).Len()
		result := make([]int64, 0, len)

		if t == reflect.Array || t == reflect.Slice {
			for i := 0; i < len; i++ {
				value := reflect.ValueOf(r).Index(i).Interface()
				result = append(result, int64(InterfaceToInt(value)))
			}
		}
		return result
	}
}

func MapGet[T any](m map[string]any, key string) T {
	var zero T
	return MapGetOr[T](m, key, zero)
}

func MapGetOr[T any](m map[string]any, key string, value T) T {
	if m == nil {
		return value
	}

	r, ok := m[key]
	if ok {
		v, typeOk := r.(T)
		if typeOk {
			return v
		}
	}
	return value
}

func InterfaceToGeneralMap(params interface{}) (finalResult map[string]interface{}) {
	if IsNil(params) {
		return map[string]any{}
	}

	defer func() {
		if err := recover(); err != nil {
			log.Errorf("handle ptr/struct to map failed: %s", err)
			finalResult = map[string]interface{}{
				"__FALLBACK__": params,
			}
		}
	}()

	p := map[string]interface{}{}
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

func ToMapParams(params any) (map[string]any, error) {
	p := map[string]any{}
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

func ParseStringToGeneralMap(i any) map[string]any {
	data := InterfaceToString(i)
	data = strings.TrimSpace(data)
	var target any
	err := json.Unmarshal([]byte(data), &target)
	if err != nil {
		log.Warnf("parse `%v` to map[string]any failed: %s", data, err)
		return make(map[string]any)
	}
	return InterfaceToGeneralMap(target)
}

func MergeToMap[K comparable, V any](m map[K]V, ms ...map[K]V) {
	for _, i := range ms {
		for k, v := range i {
			m[k] = v
		}
	}
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

func MergeGeneralMap(ms ...map[string]any) map[string]any {
	res := map[string]any{}
	for _, m := range ms {
		for k, v := range m {
			res[k] = v
		}
	}
	return res
}

func MapToStruct(input map[string]interface{}, output interface{}) error {
	outputValue := reflect.ValueOf(output)
	if outputValue.Kind() != reflect.Ptr || outputValue.IsNil() {
		return fmt.Errorf("output must be a non-nil pointer to a struct")
	}

	outputType := outputValue.Elem().Type()

	for i := 0; i < outputType.NumField(); i++ {
		field := outputType.Field(i)
		fieldName := field.Tag.Get("json")

		if fieldName == "" {
			fieldName = field.Name
		}

		value, ok := input[fieldName]
		if !ok {
			continue
		}

		fieldValue := outputValue.Elem().FieldByName(field.Name)
		if !fieldValue.IsValid() {
			continue
		}

		if fieldValue.CanSet() {
			fieldValue.Set(reflect.ValueOf(value))
		}
	}

	return nil
}
