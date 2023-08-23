package dap

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"unsafe"

	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

func SafeReflectValue(refV reflect.Value) reflect.Value {
	if !refV.CanAddr() {
		newRefV := reflect.New(refV.Type()).Elem()
		newRefV.Set(refV)
		refV = newRefV
	}
	return refV
}

func SafeReflectStructFieldInterface(refV reflect.Value, field reflect.Value) interface{} {
	if refV.CanAddr() {
		field := reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem()
		return field.Interface()
	}
	return nil
}

func AsDebugString(i interface{}, raws ...bool) string {
	refV := reflect.ValueOf(i)
	if !refV.IsValid() {
		return fmt.Sprintf("%#v", i)
	}
	typ := refV.Type()
	kind := typ.Kind()
	raw := false
	if len(raws) > 0 && raws[0] {
		raw = true
	}
	// todo: 美化输出,主要使用省略号来省略过长的字符串
	_ = raw

	if !refV.IsValid() {
		return fmt.Sprintf("%#v", i)
	}

	if kind == reflect.Array || kind == reflect.Slice {
		length := refV.Len()
		if length > 0 {
			elemKind := refV.Index(0).Kind()
			if elemKind == reflect.Uint8 { // []byte
				v := i.([]byte)
				return string(v)
			} else if elemKind == reflect.Int32 { // []rune
				v := i.([]rune)
				return string(v)
			}
		}
		content := make([]string, length)
		for i := 0; i < length; i++ {
			content[i] = AsDebugString(refV.Index(i).Interface())
		}
		return fmt.Sprintf("%T{%s}", i, strings.Join(content, ", "))
	} else if kind == reflect.String {
		s := i.(string)
		return fmt.Sprintf("%q", s)
	} else if kind == reflect.Map {
		content := make([]string, refV.Len())
		keys := refV.MapKeys()
		sort.SliceStable(keys, func(i, j int) bool {
			return fmt.Sprintf("%v", keys[i]) < fmt.Sprintf("%v", keys[j])
		})
		for i, key := range keys {
			content[i] = fmt.Sprintf("%s: %s", AsDebugString(key.Interface()), AsDebugString(refV.MapIndex(key).Interface()))
		}
		return fmt.Sprintf("%T{%s}", i, strings.Join(content, ", "))
	} else if f, ok := i.(*yakvm.Function); ok {
		return fmt.Sprintf("%s", f.String())
	} else if kind == reflect.Ptr {
		elem := refV.Elem()
		if elem.IsValid() {
			return fmt.Sprintf("&%s", AsDebugString(elem.Interface()))
		} else {
			return fmt.Sprintf("%#v", i)
		}
	} else if kind == reflect.Struct {
		content := make([]string, refV.NumField())
		if !refV.CanAddr() {
			newRefV := reflect.New(refV.Type()).Elem()
			newRefV.Set(refV)
			refV = newRefV
		}
		for i := 0; i < refV.NumField(); i++ {
			field := refV.Field(i)
			fieldStr := "<Unavailable>"
			iField := SafeReflectStructFieldInterface(refV, field)
			if iField != nil {
				fieldStr = AsDebugString(iField)
			}
			content[i] = fmt.Sprintf("%s: %s", typ.Field(i).Name, fieldStr)
		}
		return fmt.Sprintf("%T{%s}", i, strings.Join(content, ", "))
	}
	return fmt.Sprintf("%#v", i)
}
