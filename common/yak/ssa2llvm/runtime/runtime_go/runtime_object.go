package main

/*
#include <stdint.h>
void yak_invoke_callable(uintptr_t fn, void* ctx);
*/
import "C"

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"math"
	"os"
	"reflect"
	"strings"
	"unicode"
	"unsafe"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
)

func runtimeCStringToGoString(ptr unsafe.Pointer) string {
	if ptr == nil {
		return ""
	}
	return C.GoString((*C.char)(ptr))
}

func runtimeDispatchShadowMethod(args []uint64, _ bool) (int64, error) {
	if len(args) < 2 {
		return 0, fmt.Errorf("runtime shadow method expects at least 2 args, got %d", len(args))
	}

	methodNamePtr := unsafe.Pointer(uintptr(args[1]))
	if methodNamePtr == nil {
		return 0, fmt.Errorf("runtime shadow method missing method name")
	}

	objPtr := unsafe.Pointer(uintptr(args[0]))
	if objPtr == nil {
		// A nil receiver (e.g. a dropped error left the object nil) reads as
		// nil instead of panicking, matching yak's weak-typed method calls.
		return 0, nil
	}

	return callRuntimeShadowMethod(objPtr, runtimeCStringToGoString(methodNamePtr), args[2:])
}

func runtimeResolveMethod(obj any, name string) (reflect.Value, error) {
	value := reflect.ValueOf(obj)
	if !value.IsValid() {
		return reflect.Value{}, fmt.Errorf("invalid object while resolving method %q", name)
	}

	if method := value.MethodByName(name); method.IsValid() {
		return method, nil
	}

	if f, ok := obj.(*os.File); ok {
		if method, ok := runtimeResolveOSFileMethod(f, name); ok {
			return method, nil
		}
	}

	for value.IsValid() && value.Kind() == reflect.Interface {
		if value.IsNil() {
			break
		}
		value = value.Elem()
		if method := value.MethodByName(name); method.IsValid() {
			return method, nil
		}
	}

	if value.IsValid() && value.Kind() == reflect.String {
		if method, ok := runtimeResolveStringMethod(value.String(), name); ok {
			return method, nil
		}
	}

	if value.IsValid() && (value.Kind() == reflect.Slice || value.Kind() == reflect.Array) {
		if method, ok := runtimeResolveSliceMethod(value, name); ok {
			return method, nil
		}
	}
	if value.IsValid() && value.Kind() == reflect.Ptr && !value.IsNil() {
		elem := value.Elem()
		if elem.IsValid() && (elem.Kind() == reflect.Slice || elem.Kind() == reflect.Array) {
			if method, ok := runtimeResolveSliceMethod(value, name); ok {
				return method, nil
			}
		}
	}

	if value.IsValid() && value.Kind() != reflect.Ptr && value.CanAddr() {
		if method := value.Addr().MethodByName(name); method.IsValid() {
			return method, nil
		}
	}

	return reflect.Value{}, fmt.Errorf("method %q not found", name)
}

// runtimeResolveOSFileMethod implements the yak file methods that os.File
// does not provide natively (WriteLine/ReadLines/Tell/...).
func runtimeResolveOSFileMethod(f *os.File, name string) (reflect.Value, bool) {
	if f == nil {
		return reflect.Value{}, false
	}
	switch name {
	case "WriteLine":
		return reflect.ValueOf(func(line any) (int, error) {
			return f.WriteString(fmt.Sprintf("%v\n", line))
		}), true
	case "ReadLines":
		return reflect.ValueOf(func() []string {
			lines := make([]string, 0)
			sc := bufio.NewScanner(f)
			for sc.Scan() {
				lines = append(lines, sc.Text())
			}
			return lines
		}), true
	case "ReadLine":
		return reflect.ValueOf(func() (string, error) {
			sc := bufio.NewScanner(f)
			if sc.Scan() {
				return sc.Text(), nil
			}
			if err := sc.Err(); err != nil {
				return "", err
			}
			return "", io.EOF
		}), true
	case "Seek":
		return reflect.ValueOf(func(offset int64, whence int) (int64, error) {
			return f.Seek(offset, whence)
		}), true
	case "Tell":
		return reflect.ValueOf(func() (int64, error) {
			return f.Seek(0, io.SeekCurrent)
		}), true
	case "ReadAll":
		return reflect.ValueOf(func() ([]byte, error) { return io.ReadAll(f) }), true
	case "ReadString":
		return reflect.ValueOf(func() (string, error) {
			b, err := io.ReadAll(f)
			return string(b), err
		}), true
	case "Write":
		return reflect.ValueOf(func(data any) (int, error) {
			switch d := data.(type) {
			case string:
				return f.WriteString(d)
			case []byte:
				return f.Write(d)
			default:
				return f.WriteString(fmt.Sprint(d))
			}
		}), true
	case "WriteString":
		return reflect.ValueOf(func(s string) (int, error) { return f.WriteString(s) }), true
	case "Name":
		return reflect.ValueOf(func() string { return f.Name() }), true
	}
	return reflect.Value{}, false
}

// runtimeResolveSliceMethod implements the yak slice/array methods that the
// AOT runtime must provide (Go slices have no methods of their own). The
// receiver may be a *[]T shadow (yak reference semantics) or a plain []T.
func runtimeResolveSliceMethod(v reflect.Value, name string) (reflect.Value, bool) {
	ptr := reflect.Value{}
	for v.IsValid() && v.Kind() == reflect.Interface {
		if v.IsNil() {
			return reflect.Value{}, false
		}
		v = v.Elem()
	}
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return reflect.Value{}, false
		}
		ptr = v
		v = v.Elem()
	}
	if !v.IsValid() || (v.Kind() != reflect.Slice && v.Kind() != reflect.Array) {
		return reflect.Value{}, false
	}
	switch name {
	case "Append", "Push":
		return reflect.ValueOf(func(items ...any) any {
			elems := make([]reflect.Value, 0, len(items))
			for _, item := range items {
				elems = append(elems, runtimeSliceAppendValue(v.Type().Elem(), item))
			}
			appended := reflect.Append(v, elems...)
			if ptr.IsValid() {
				ptr.Elem().Set(appended)
				return ptr.Interface()
			}
			return appended.Interface()
		}), true
	case "Map":
		return reflect.ValueOf(func(fn any) []any {
			out := make([]any, 0, v.Len())
			for i := 0; i < v.Len(); i++ {
				out = append(out, runtimeCallMappedElement(fn, v.Index(i).Interface()))
			}
			return out
		}), true
	case "Length", "Len":
		return reflect.ValueOf(func() int { return v.Len() }), true
	case "Get", "At":
		return reflect.ValueOf(func(i int) any {
			if i < 0 || i >= v.Len() {
				return nil
			}
			return v.Index(i).Interface()
		}), true
	case "Set":
		return reflect.ValueOf(func(i int, item any) {
			if i < 0 || i >= v.Len() {
				return
			}
			converted, ok := valueForSet(v.Type().Elem(), int64(0))
			_ = converted
			_ = ok
			// Use the same conversion path as append for consistency.
			rv := runtimeSliceAppendValue(v.Type().Elem(), item)
			v.Index(i).Set(rv)
		}), true
	case "Contains":
		return reflect.ValueOf(func(item any) bool {
			// []byte.Contains("...") matches bytes.Contains semantics.
			if v.IsValid() && v.Kind() == reflect.Slice && v.Type().Elem().Kind() == reflect.Uint8 {
				raw := v.Bytes()
				switch sub := item.(type) {
				case string:
					return bytes.Contains(raw, []byte(sub))
				case []byte:
					return bytes.Contains(raw, sub)
				}
			}
			for i := 0; i < v.Len(); i++ {
				if runtimeValuesEqual(v.Index(i).Interface(), item) {
					return true
				}
			}
			return false
		}), true
	}
	return reflect.Value{}, false
}

func runtimeResolveStringMethod(s string, name string) (reflect.Value, bool) {
	switch name {
	case "Trim":
		return reflect.ValueOf(func(cutset ...string) string {
			if len(cutset) == 0 {
				return strings.TrimSpace(s)
			}
			return strings.Trim(s, strings.Join(cutset, ""))
		}), true
	case "TrimLeft":
		return reflect.ValueOf(func(cutset ...string) string {
			if len(cutset) == 0 {
				return strings.TrimLeftFunc(s, unicode.IsSpace)
			}
			return strings.TrimLeft(s, strings.Join(cutset, ""))
		}), true
	case "TrimRight":
		return reflect.ValueOf(func(cutset ...string) string {
			if len(cutset) == 0 {
				return strings.TrimRightFunc(s, unicode.IsSpace)
			}
			return strings.TrimRight(s, strings.Join(cutset, ""))
		}), true
	case "Lower":
		return reflect.ValueOf(func() string { return strings.ToLower(s) }), true
	case "Upper":
		return reflect.ValueOf(func() string { return strings.ToUpper(s) }), true
	case "Contains":
		return reflect.ValueOf(func(substr string) bool { return substr == "" || strings.Contains(s, substr) }), true
	case "HasPrefix", "StartsWith":
		return reflect.ValueOf(func(prefix string) bool { return strings.HasPrefix(s, prefix) }), true
	case "HasSuffix", "EndsWith":
		return reflect.ValueOf(func(suffix string) bool { return strings.HasSuffix(s, suffix) }), true
	case "RemovePrefix":
		return reflect.ValueOf(func(prefix string) string {
			return strings.TrimPrefix(s, prefix)
		}), true
	case "RemoveSuffix":
		return reflect.ValueOf(func(suffix string) string {
			return strings.TrimSuffix(s, suffix)
		}), true
	case "Split":
		return reflect.ValueOf(func(sep string) []string { return strings.Split(s, sep) }), true
	case "SplitN":
		return reflect.ValueOf(func(sep string, n int) []string { return strings.SplitN(s, sep, n) }), true
	case "Count":
		return reflect.ValueOf(func(substr string) int { return strings.Count(s, substr) }), true
	case "Find", "IndexOf":
		return reflect.ValueOf(func(substr string) int { return strings.Index(s, substr) }), true
	case "Rfind", "LastIndexOf":
		return reflect.ValueOf(func(substr string) int { return strings.LastIndex(s, substr) }), true
	case "Join":
		return reflect.ValueOf(func(items []any) string {
			strs := make([]string, 0, len(items))
			for _, item := range items {
				strs = append(strs, fmt.Sprint(item))
			}
			return strings.Join(strs, s)
		}), true
	default:
		return reflect.Value{}, false
	}
}

type runtimeCallableClosure struct {
	fn               uint64
	paramMemberCount int
	freeValues       []uint64
}

func runtimeDecodeArg(raw uint64, targetType reflect.Type) (reflect.Value, error) {
	if targetType == nil {
		return reflect.Value{}, fmt.Errorf("missing target type")
	}

	// Numeric targets: the raw word is the value itself. decodeTaggedArg's
	// C-string fallback would misread a large integer (e.g. a nanosecond
	// duration like 5e9) as a pointer and crash.
	switch targetType.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return reflect.ValueOf(int64(raw &^ yakTaggedPointerMask)).Convert(targetType), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return reflect.ValueOf(raw &^ yakTaggedPointerMask).Convert(targetType), nil
	case reflect.Float32, reflect.Float64:
		bits := raw &^ yakTaggedPointerMask
		// The compiler encodes float constants as float64 bit patterns and int
		// constants as int64 words. Reinterpret the bits when they form a
		// normal float (large word); otherwise treat the word as an integer.
		if f := math.Float64frombits(bits); !math.IsInf(f, 0) && !math.IsNaN(f) &&
			math.Abs(f) >= 1e-300 && math.Abs(f) <= 1e300 && bits > 1<<32 {
			return reflect.ValueOf(f).Convert(targetType), nil
		}
		return reflect.ValueOf(float64(int64(bits))).Convert(targetType), nil
	case reflect.Bool:
		return reflect.ValueOf(raw&^yakTaggedPointerMask != 0).Convert(targetType), nil
	}

	// nil (0) is a valid value for nullable container targets. Interfaces are
	// intentionally excluded: a zero raw for an interface target must still go
	// through decodeTaggedArg so nil/error semantics stay unchanged.
	if raw == 0 {
		switch targetType.Kind() {
		case reflect.Ptr, reflect.Slice, reflect.Map, reflect.Func, reflect.Chan:
			return reflect.Zero(targetType), nil
		}
	}

	if targetType.Kind() == reflect.Func {
		if fn, ok := runtimeDecodeCallableArg(raw, targetType); ok {
			return fn, nil
		}
	}

	decoded := decodeTaggedArg(raw)
	if decoded == nil {
		return reflect.Zero(targetType), nil
	}

	if intValue, ok := decoded.(int64); ok {
		if shadowValue, ok := runtimeDecodeShadowArg(raw, targetType); ok {
			return shadowValue, nil
		}
		if converted, ok := valueForSet(targetType, intValue); ok {
			return converted, nil
		}
	}

	value := reflect.ValueOf(decoded)
	if value.IsValid() {
		if targetType.Kind() == reflect.Interface {
			// Yak maps are AOT-internal runtimeOrderedMap values; yaklib
			// expects real Go maps (utils.IsMap / range), so convert before
			// crossing the interface boundary.
			if om, ok := decoded.(*runtimeOrderedMap); ok && om != nil {
				m := make(map[string]any, len(om.keys))
				for _, k := range om.keys {
					m[k] = om.values[k]
				}
				return reflect.ValueOf(m), nil
			}
			// AOT slice/map shadows are pointers; yak code sees them as
			// values, so pass the dereferenced container to yaklib before the
			// generic assignable-to-interface path.
			if value.Kind() == reflect.Ptr && !value.IsNil() {
				elem := value.Elem()
				if elem.IsValid() && (elem.Kind() == reflect.Slice || elem.Kind() == reflect.Map) {
					return elem, nil
				}
			}
			if value.Type().Implements(targetType) {
				return value, nil
			}
		}
		if value.Type().AssignableTo(targetType) {
			return value, nil
		}
		if value.Type().ConvertibleTo(targetType) {
			return value.Convert(targetType), nil
		}
		if targetType.Kind() == reflect.Ptr && value.Kind() != reflect.Ptr && value.CanAddr() && value.Addr().Type().AssignableTo(targetType) {
			return value.Addr(), nil
		}
		if value.Kind() == reflect.Ptr && !value.IsNil() && value.Elem().Type().AssignableTo(targetType) {
			return value.Elem(), nil
		}
		if targetType.Kind() == reflect.Map {
			if converted, ok := convertMapValue(value, targetType); ok {
				return converted, nil
			}
		}
		if targetType.Kind() == reflect.Slice {
			if converted, ok := convertSliceValue(value, targetType); ok {
				return converted, nil
			}
		}
	}

	return reflect.Value{}, fmt.Errorf("cannot use %T as %s", decoded, targetType)
}

func runtimeDecodeCallableArg(raw uint64, targetType reflect.Type) (reflect.Value, bool) {
	if targetType == nil || targetType.Kind() != reflect.Func {
		return reflect.Value{}, false
	}
	if (raw & yakTaggedPointerMask) != 0 {
		raw &^= yakTaggedPointerMask
	}
	ptr := unsafe.Pointer(uintptr(raw))
	if ptr == nil {
		return reflect.Zero(targetType), true
	}
	if handle, ok := handleFromShadow(ptr); ok {
		handleValue := handle.Value()
		if closure, ok := runtimeCallableClosureValue(handleValue); ok {
			return runtimeMakeCallableWrapper(closure.fn, closure.paramMemberCount, closure.freeValues, targetType), true
		}
		value := reflect.ValueOf(handleValue)
		if value.IsValid() && value.Type().AssignableTo(targetType) {
			return value, true
		}
		if value.IsValid() && value.Type().ConvertibleTo(targetType) {
			return value.Convert(targetType), true
		}
	}

	return runtimeMakeCallableWrapper(raw, 0, nil, targetType), true
}

func runtimeCallableClosureValue(value any) (runtimeCallableClosure, bool) {
	switch closure := value.(type) {
	case runtimeCallableClosure:
		return closure, true
	case *runtimeCallableClosure:
		if closure != nil {
			return *closure, true
		}
	}
	return runtimeCallableClosure{}, false
}

func runtimeMakeCallableWrapper(raw uint64, paramMemberCount int, freeValues []uint64, targetType reflect.Type) reflect.Value {
	captures := append([]uint64(nil), freeValues...)
	return reflect.MakeFunc(targetType, func(args []reflect.Value) []reflect.Value {
		if raw == abi.YaklibExportCallableMarker && len(captures) >= 2 {
			callArgs := make([]uint64, 0, len(args)+2)
			callArgs = append(callArgs, captures[0], captures[1])
			for _, arg := range args {
				callArgs = append(callArgs, uint64(runtimeValueToInt64(arg)))
			}
			ret, err := runtimeDispatchYaklibCall(callArgs, false)
			if err != nil {
				panic(err)
			}
			return runtimeDecodeYaklibExportReturns(ret, targetType)
		}
		paramc := len(args)
		argc := paramc + paramMemberCount + len(captures)
		words := make([]uint64, abi.HeaderWords+argc*2)
		ctx := unsafe.Pointer(&words[0])
		ctxInit(ctx, abi.KindCallable, raw, argc)
		for i, arg := range args {
			rawArg := uint64(runtimeValueToInt64(arg))
			runtimeStoreCallableContextArg(ctx, argc, i, rawArg)
		}
		for i := 0; i < paramMemberCount; i++ {
			runtimeStoreCallableContextArg(ctx, argc, paramc+i, 0)
		}
		for i, capture := range captures {
			runtimeStoreCallableContextArg(ctx, argc, paramc+paramMemberCount+i, capture)
		}

		C.yak_invoke_callable(C.uintptr_t(raw), ctx)
		return runtimeDecodeCallableReturns(ctx, targetType)
	})
}

func runtimeStoreCallableContextArg(ctx unsafe.Pointer, argc int, index int, raw uint64) {
	ctxStoreWord(ctx, abi.HeaderWords+index, raw)
	ctxStoreWord(ctx, abi.HeaderWords+argc+index, raw&^yakTaggedPointerMask)
}

// runtimeDecodeYaklibExportReturns decodes a single yaklib export return value
// into the Go func wrapper's result values.
func runtimeDecodeYaklibExportReturns(ret int64, targetType reflect.Type) []reflect.Value {
	if targetType == nil || targetType.NumOut() == 0 {
		return nil
	}
	out := make([]reflect.Value, targetType.NumOut())
	for i := range out {
		if i == 0 {
			if value, err := runtimeDecodeArg(uint64(ret), targetType.Out(i)); err == nil {
				out[i] = value
				continue
			}
		}
		out[i] = reflect.Zero(targetType.Out(i))
	}
	return out
}

func runtimeDecodeCallableReturns(ctx unsafe.Pointer, targetType reflect.Type) []reflect.Value {
	if targetType == nil || targetType.NumOut() == 0 {
		return nil
	}

	out := make([]reflect.Value, targetType.NumOut())
	ret := ctxLoadWord(ctx, abi.WordRet)
	for i := range out {
		if i == 0 {
			if value, err := runtimeDecodeArg(ret, targetType.Out(i)); err == nil {
				out[i] = value
				continue
			}
		}
		out[i] = reflect.Zero(targetType.Out(i))
	}
	return out
}

func runtimeDecodeShadowArg(raw uint64, targetType reflect.Type) (reflect.Value, bool) {
	ptr := unsafe.Pointer(uintptr(raw))
	if ptr == nil {
		return reflect.Value{}, false
	}

	handle, ok := handleFromShadow(ptr)
	if !ok {
		return reflect.Value{}, false
	}

	value := reflect.ValueOf(handle.Value())
	if !value.IsValid() {
		return reflect.Zero(targetType), true
	}
	if value.Type().AssignableTo(targetType) {
		return value, true
	}
	if value.Type().ConvertibleTo(targetType) {
		return value.Convert(targetType), true
	}
	if targetType.Kind() == reflect.Interface && value.Type().Implements(targetType) {
		return value, true
	}
	if targetType.Kind() == reflect.Ptr && value.Kind() != reflect.Ptr && value.CanAddr() && value.Addr().Type().AssignableTo(targetType) {
		return value.Addr(), true
	}
	if targetType.Kind() == reflect.Map {
		if converted, ok := convertMapValue(value, targetType); ok {
			return converted, true
		}
	}
	if targetType.Kind() == reflect.Slice {
		if converted, ok := convertSliceValue(value, targetType); ok {
			return converted, true
		}
	}
	return reflect.Value{}, false
}

// convertMapValue converts an AOT runtimeOrderedMap (or a Go map) into the
// target map type yaklib expects (e.g. map[string]interface{}), copying keys
// and values element-wise.
func convertMapValue(value reflect.Value, targetType reflect.Type) (reflect.Value, bool) {
	if !value.IsValid() || targetType == nil || targetType.Kind() != reflect.Map {
		return reflect.Value{}, false
	}
	var keys []string
	var values map[string]any
	if om, ok := value.Interface().(*runtimeOrderedMap); ok && om != nil {
		keys = om.keys
		values = om.values
	} else if value.Kind() == reflect.Ptr {
		if value.IsNil() {
			return reflect.Value{}, false
		}
		if om, ok := value.Interface().(*runtimeOrderedMap); ok && om != nil {
			keys = om.keys
			values = om.values
		} else {
			value = value.Elem()
			if !value.IsValid() || value.Kind() != reflect.Map {
				return reflect.Value{}, false
			}
			keys = make([]string, 0, value.Len())
			values = make(map[string]any, value.Len())
			iter := value.MapRange()
			for iter.Next() {
				k := fmt.Sprint(iter.Key().Interface())
				keys = append(keys, k)
				values[k] = iter.Value().Interface()
			}
		}
	} else if value.Kind() == reflect.Map {
		keys = make([]string, 0, value.Len())
		values = make(map[string]any, value.Len())
		iter := value.MapRange()
		for iter.Next() {
			k := fmt.Sprint(iter.Key().Interface())
			keys = append(keys, k)
			values[k] = iter.Value().Interface()
		}
	} else {
		return reflect.Value{}, false
	}

	keyType := targetType.Key()
	elemType := targetType.Elem()
	out := reflect.MakeMapWithSize(targetType, len(keys))
	for _, k := range keys {
		kv := reflect.ValueOf(k)
		if !kv.Type().AssignableTo(keyType) {
			if !kv.Type().ConvertibleTo(keyType) {
				return reflect.Value{}, false
			}
			kv = kv.Convert(keyType)
		}
		ev := reflect.ValueOf(values[k])
		if !ev.IsValid() {
			ev = reflect.Zero(elemType)
		}
		if !ev.Type().AssignableTo(elemType) {
			if !ev.Type().ConvertibleTo(elemType) {
				return reflect.Value{}, false
			}
			ev = ev.Convert(elemType)
		}
		out.SetMapIndex(kv, ev)
	}
	return out, true
}

// convertSliceValue converts a Go slice (usually []any) into the target slice
// type yaklib expects, converting elements one by one.
func convertSliceValue(value reflect.Value, targetType reflect.Type) (reflect.Value, bool) {
	if !value.IsValid() || targetType == nil || targetType.Kind() != reflect.Slice {
		return reflect.Value{}, false
	}
	if value.Kind() == reflect.Ptr {
		if value.IsNil() {
			return reflect.Value{}, false
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Slice {
		return reflect.Value{}, false
	}
	if value.Type().AssignableTo(targetType) {
		return value, true
	}
	elemType := targetType.Elem()
	out := reflect.MakeSlice(targetType, value.Len(), value.Len())
	for i := 0; i < value.Len(); i++ {
		elem := value.Index(i)
		for elem.IsValid() && elem.Kind() == reflect.Interface {
			if elem.IsNil() {
				elem = reflect.Zero(elemType)
				break
			}
			elem = elem.Elem()
		}
		if !elem.IsValid() {
			out.Index(i).Set(reflect.Zero(elemType))
			continue
		}
		if elem.Type().AssignableTo(elemType) {
			out.Index(i).Set(elem)
			continue
		}
		if elem.Type().ConvertibleTo(elemType) {
			out.Index(i).Set(elem.Convert(elemType))
			continue
		}
		if elemType.Kind() == reflect.String {
			out.Index(i).SetString(fmt.Sprint(elem.Interface()))
			continue
		}
		return reflect.Value{}, false
	}
	return out, true
}

func convertSliceForVariadicCall(val reflect.Value, targetSliceType reflect.Type) (reflect.Value, bool) {
	if !val.IsValid() || val.Kind() != reflect.Slice || targetSliceType == nil || targetSliceType.Kind() != reflect.Slice {
		return reflect.Value{}, false
	}
	if val.Type().AssignableTo(targetSliceType) {
		return val, true
	}
	elemType := targetSliceType.Elem()
	out := reflect.MakeSlice(targetSliceType, val.Len(), val.Len())
	for i := 0; i < val.Len(); i++ {
		elem := val.Index(i)
		for elem.IsValid() && elem.Kind() == reflect.Interface {
			if elem.IsNil() {
				elem = reflect.Zero(elemType)
				break
			}
			elem = elem.Elem()
		}
		if !elem.IsValid() {
			out.Index(i).Set(reflect.Zero(elemType))
			continue
		}
		if elem.Type().AssignableTo(elemType) {
			out.Index(i).Set(elem)
			continue
		}
		if elem.Type().ConvertibleTo(elemType) {
			out.Index(i).Set(elem.Convert(elemType))
			continue
		}
		if elemType.Kind() == reflect.String {
			out.Index(i).SetString(fmt.Sprint(elem.Interface()))
			continue
		}
		return reflect.Value{}, false
	}
	return out, true
}

func runtimeDecodeCallArgs(target reflect.Value, rawArgs []uint64, ellipsis bool) ([]reflect.Value, error) {
	methodType := target.Type()
	if !methodType.IsVariadic() {
		if len(rawArgs) != methodType.NumIn() {
			return nil, fmt.Errorf("method expects %d args, got %d", methodType.NumIn(), len(rawArgs))
		}
		args := make([]reflect.Value, 0, len(rawArgs))
		for index, raw := range rawArgs {
			arg, err := runtimeDecodeArg(raw, methodType.In(index))
			if err != nil {
				return nil, err
			}
			args = append(args, arg)
		}
		return args, nil
	}

	fixedCount := methodType.NumIn() - 1
	variadicSliceType := methodType.In(fixedCount)
	variadicElemType := variadicSliceType.Elem()

	decodeFixed := func() ([]reflect.Value, error) {
		out := make([]reflect.Value, 0, fixedCount)
		for i := 0; i < fixedCount; i++ {
			arg, err := runtimeDecodeArg(rawArgs[i], methodType.In(i))
			if err != nil {
				return nil, err
			}
			out = append(out, arg)
		}
		return out, nil
	}

	if len(rawArgs) < fixedCount {
		return nil, fmt.Errorf("method expects at least %d args, got %d", fixedCount, len(rawArgs))
	}

	fixed, err := decodeFixed()
	if err != nil {
		return nil, err
	}

	variadicArgs := rawArgs[fixedCount:]

	// `f(slice...)` unpacks the slice into variadic elements; `f(slice)`
	// passes the slice as a single element. The compiler marks ellipsis calls
	// with FlagEllipsis so the two are distinguishable at runtime.
	if ellipsis && len(variadicArgs) == 1 {
		if sliceVal, ok := runtimeDecodeVariadicSliceValue(variadicArgs[0]); ok {
			elems := make([]reflect.Value, 0, sliceVal.Len())
			for i := 0; i < sliceVal.Len(); i++ {
				e := sliceVal.Index(i)
				for e.IsValid() && e.Kind() == reflect.Interface {
					if e.IsNil() {
						e = reflect.Zero(variadicElemType)
						break
					}
					e = e.Elem()
				}
				if e.IsValid() && e.Kind() == reflect.Ptr && !e.IsNil() {
					ee := e.Elem()
					if ee.IsValid() && (ee.Kind() == reflect.Slice || ee.Kind() == reflect.Map) {
						e = ee
					}
				}
				elems = append(elems, e)
			}
			slice := reflect.MakeSlice(variadicSliceType, len(elems), len(elems))
			for i, elem := range elems {
				slice.Index(i).Set(elem)
			}
			return append(fixed, slice), nil
		}
	}

	if len(variadicArgs) == 0 {
		return append(fixed, reflect.Zero(variadicSliceType)), nil
	}

	elems := make([]reflect.Value, 0, len(variadicArgs))
	for _, raw := range variadicArgs {
		arg, err := runtimeDecodeArg(raw, variadicElemType)
		if err != nil {
			return nil, err
		}
		elems = append(elems, arg)
	}
	slice := reflect.MakeSlice(variadicSliceType, len(elems), len(elems))
	for i, elem := range elems {
		slice.Index(i).Set(elem)
	}
	return append(fixed, slice), nil
}

// runtimeDecodeVariadicSliceValue resolves a single raw argument that is an
// AOT slice shadow (*[]T) into its underlying slice reflect.Value.
func runtimeDecodeVariadicSliceValue(raw uint64) (reflect.Value, bool) {
	ptr := unsafe.Pointer(uintptr(raw &^ yakTaggedPointerMask))
	if ptr == nil {
		return reflect.Value{}, false
	}
	handle, ok := handleFromShadow(ptr)
	if !ok {
		return reflect.Value{}, false
	}
	value := reflect.ValueOf(handle.Value())
	for value.IsValid() && value.Kind() == reflect.Interface {
		if value.IsNil() {
			return reflect.Value{}, false
		}
		value = value.Elem()
	}
	if value.Kind() == reflect.Ptr {
		if value.IsNil() {
			return reflect.Value{}, false
		}
		value = value.Elem()
	}
	if !value.IsValid() || value.Kind() != reflect.Slice {
		return reflect.Value{}, false
	}
	return value, true
}

func runtimeCallReturnValue(results []reflect.Value) int64 {
	if len(results) == 0 {
		return 0
	}

	if len(results) == 1 {
		return runtimeValueToInt64(results[0])
	}

	// Multi-return Yak calls unpack via tuple index ("0", "1", ...). Box every
	// result into a []any shadow so runtime get_field can read each slot.
	tuple := make([]any, len(results))
	for i, r := range results {
		if !r.IsValid() || (r.Kind() == reflect.Interface && r.IsNil()) {
			tuple[i] = nil
			continue
		}
		tuple[i] = r.Interface()
	}
	return int64(uintptr(newRuntimeShadow(tuple)))
}

func callRuntimeValue(target reflect.Value, rawArgs []uint64, ellipsis bool) (int64, error) {
	args, err := runtimeDecodeCallArgs(target, rawArgs, ellipsis)
	if err != nil {
		return 0, err
	}
	methodType := target.Type()
	if methodType.IsVariadic() && len(args) > 0 {
		last := args[len(args)-1]
		unwrapped := last
		for unwrapped.IsValid() && unwrapped.Kind() == reflect.Interface {
			if unwrapped.IsNil() {
				break
			}
			unwrapped = unwrapped.Elem()
		}
		if unwrapped.IsValid() && unwrapped.Kind() == reflect.Ptr && !unwrapped.IsNil() {
			unwrapped = unwrapped.Elem()
		}
		if unwrapped.IsValid() && unwrapped.Kind() == reflect.Slice {
			// For interface{} variadic targets, unwrap AOT slice/map shadow
			// elements so yaklib sees the container values, not pointers.
			if methodType.NumIn() > 0 && methodType.In(methodType.NumIn()-1).Kind() == reflect.Interface {
				for i := 0; i < unwrapped.Len(); i++ {
					e := unwrapped.Index(i)
					for e.IsValid() && e.Kind() == reflect.Interface {
						if e.IsNil() {
							break
						}
						e = e.Elem()
					}
					if e.IsValid() && e.Kind() == reflect.Ptr && !e.IsNil() {
						ee := e.Elem()
						if ee.IsValid() && (ee.Kind() == reflect.Slice || ee.Kind() == reflect.Map) {
							unwrapped.Index(i).Set(ee)
						}
					}
				}
			}
			args[len(args)-1] = unwrapped
			return runtimeCallReturnValue(target.CallSlice(args)), nil
		}
	}
	return runtimeCallReturnValue(target.Call(args)), nil
}

func callRuntimeShadowMethod(objPtr unsafe.Pointer, methodName string, rawArgs []uint64) (int64, error) {
	handle, ok := handleFromShadow(objPtr)
	if !ok {
		// String receivers are passed as C-string pointers (or string shadows)
		// rather than shadow handles; resolve the string method directly.
		if s, ok := tryResolveShadowString(objPtr); ok {
			if method, ok := runtimeResolveStringMethod(s, methodName); ok {
				return callRuntimeValue(method, rawArgs, false)
			}
			return 0, fmt.Errorf("method %q not found on string", methodName)
		}
		raw := uint64(uintptr(objPtr))
		raw &^= yakTaggedPointerMask
		if looksLikeCStringPointer(raw) {
			s := runtimeCStringToGoString(unsafe.Pointer(uintptr(raw)))
			if method, ok := runtimeResolveStringMethod(s, methodName); ok {
				return callRuntimeValue(method, rawArgs, false)
			}
			return 0, fmt.Errorf("method %q not found on string", methodName)
		}
		return 0, fmt.Errorf("invalid shadow object for method %q", methodName)
	}

	method, err := runtimeResolveMethod(handle.Value(), methodName)
	if err != nil {
		return 0, err
	}

	return callRuntimeValue(method, rawArgs, false)
}

//export yak_runtime_drop_error
func yak_runtime_drop_error(value unsafe.Pointer) int64 {
	defer recoverRuntimePanic()
	if value == nil {
		return 0
	}
	if h, ok := handleFromShadow(value); ok {
		if tuple, ok := h.Value().([]any); ok && len(tuple) > 0 {
			// Multi-return tuple from a yaklib call: `f()~` keeps the first
			// value and drops the trailing error.
			return runtimeValueToInt64(reflect.ValueOf(tuple[0]))
		}
	}
	// Single-return call: the value is already the result; preserve its word.
	return int64(uintptr(value))
}

//export yak_runtime_string_slice
func yak_runtime_string_slice(parent unsafe.Pointer, low, high int64) unsafe.Pointer {
	defer recoverRuntimePanic()
	s := runtimePtrToString(parent)
	runes := []rune(s)
	if low < 0 {
		low = 0
	}
	if high < 0 || high > int64(len(runes)) {
		high = int64(len(runes))
	}
	if low > high {
		low = high
	}
	return newStdlibShadow(string(runes[low:high]))
}

//export yak_runtime_concat
func yak_runtime_concat(a, b unsafe.Pointer) unsafe.Pointer {
	defer recoverRuntimePanic()
	as := runtimePtrToString(a)
	bs := runtimePtrToString(b)
	return newStdlibShadow(as + bs)
}

func runtimePtrToString(ptr unsafe.Pointer) string {
	if ptr == nil {
		return ""
	}
	if s, ok := tryResolveShadowString(ptr); ok {
		return s
	}
	raw := uint64(uintptr(ptr))
	raw &^= yakTaggedPointerMask
	if !looksLikeCStringPointer(raw) {
		// Not a string pointer: yak template interpolation converts the value
		// to its string form (e.g. an int port becomes "41925").
		return fmt.Sprintf("%d", int64(raw))
	}
	return runtimeCStringToGoString(unsafe.Pointer(uintptr(raw)))
}

func runtimeCallMappedElement(fn any, elem any) any {
	if fn == nil {
		return elem
	}
	fv := reflect.ValueOf(fn)
	if !fv.IsValid() || fv.Kind() != reflect.Func {
		return elem
	}
	out := fv.Call([]reflect.Value{reflect.ValueOf(elem)})
	if len(out) == 0 {
		return nil
	}
	return out[0].Interface()
}
