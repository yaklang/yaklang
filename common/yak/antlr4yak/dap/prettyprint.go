package dap

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"unsafe"

	"github.com/davecgh/go-spew/spew"
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

func SafeReflectStructField(refV reflect.Value, field reflect.Value) reflect.Value {
	var f reflect.Value
	if refV.CanAddr() {
		f = reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem()
	}
	return f
}

func AsDebugString(i interface{}, raws ...bool) string {
	raw := false
	if len(raws) > 0 && raws[0] {
		raw = true
	}
	return asDebugString(i, raw, make(map[uintptr]struct{}))
}

func asDebugString(i interface{}, raw bool, pointers map[uintptr]struct{}) (ret string) {
	spew.Sdump()

	refV := reflect.ValueOf(i)
	if !refV.IsValid() {
		return fmt.Sprintf("%#v", i)
	}
	typ := refV.Type()
	kind := typ.Kind()

	// todo: 美化输出,主要使用省略号来省略过长的字符串
	_ = raw

	if !refV.IsValid() {
		return fmt.Sprintf("%#v", i)
	}

	if kind == reflect.Array || kind == reflect.Slice {
		length := refV.Len()
		if length > 0 {
			elemKind := refV.Index(0).Kind()
			if elemKind == reflect.Uint8 || elemKind == reflect.Int32 { // []byte or []rune
				return fmt.Sprintf("%q", i)
			}
		}
		content := make([]string, length)
		for i := 0; i < length; i++ {
			content[i] = asDebugString(refV.Index(i).Interface(), raw, pointers)
		}
		return fmt.Sprintf("%T{%s}", i, strings.Join(content, ", "))
	} else if kind == reflect.String {
		return fmt.Sprintf("%q", i)
	} else if kind == reflect.Map {
		content := make([]string, refV.Len())
		keys := refV.MapKeys()
		sort.SliceStable(keys, func(i, j int) bool {
			return fmt.Sprintf("%v", keys[i]) < fmt.Sprintf("%v", keys[j])
		})
		for i, key := range keys {
			content[i] = fmt.Sprintf("%s: %s", asDebugString(key.Interface(), raw, pointers), asDebugString(refV.MapIndex(key).Interface(), raw, pointers))
		}
		return fmt.Sprintf("%T{%s}", i, strings.Join(content, ", "))
	} else if f, ok := i.(*yakvm.Function); ok {
		return fmt.Sprintf("%s", f.String())
	} else if kind == reflect.Ptr {
		// fix circle call
		p := refV.Pointer()
		if _, ok := pointers[p]; ok {
			return "<Already printed>"
		} else {
			pointers[p] = struct{}{}
		}
		elem := refV.Elem()
		if elem.IsValid() {
			return fmt.Sprintf("&%s", asDebugString(elem.Interface(), raw, pointers))
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
			newField := SafeReflectStructField(refV, field)
			iField := newField.Interface()
			if iField != nil {
				fieldStr = asDebugString(iField, raw, pointers)
			}
			content[i] = fmt.Sprintf("%s: %s", typ.Field(i).Name, fieldStr)
		}
		return fmt.Sprintf("%T{%s}", i, strings.Join(content, ", "))
	} else if refV.CanInt() || refV.CanUint() {
		return fmt.Sprintf("%d", i)
	} else if refV.CanFloat() {
		return fmt.Sprintf("%f", i)
	}
	return fmt.Sprintf("%#v", i)
}
