package main

/*
#include <stdint.h>
void yak_invoke_callable(uintptr_t fn, void* ctx);
*/
import "C"

import (
	"fmt"
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

func runtimeDispatchShadowMethod(args []uint64) (int64, error) {
	if len(args) < 2 {
		return 0, fmt.Errorf("runtime shadow method expects at least 2 args, got %d", len(args))
	}

	methodNamePtr := unsafe.Pointer(uintptr(args[1]))
	if methodNamePtr == nil {
		return 0, fmt.Errorf("runtime shadow method missing method name")
	}

	objPtr := unsafe.Pointer(uintptr(args[0]))
	if objPtr == nil {
		return 0, fmt.Errorf("runtime shadow method missing receiver")
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

	if value.IsValid() && value.Kind() != reflect.Ptr && value.CanAddr() {
		if method := value.Addr().MethodByName(name); method.IsValid() {
			return method, nil
		}
	}

	return reflect.Value{}, fmt.Errorf("method %q not found", name)
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
		return reflect.ValueOf(func(prefix string) string { return strings.TrimPrefix(s, prefix) }), true
	case "RemoveSuffix":
		return reflect.ValueOf(func(suffix string) string { return strings.TrimSuffix(s, suffix) }), true
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
		if value.Type().AssignableTo(targetType) {
			return value, nil
		}
		if value.Type().ConvertibleTo(targetType) {
			return value.Convert(targetType), nil
		}
		if targetType.Kind() == reflect.Interface && value.Type().Implements(targetType) {
			return value, nil
		}
		if targetType.Kind() == reflect.Ptr && value.Kind() != reflect.Ptr && value.CanAddr() && value.Addr().Type().AssignableTo(targetType) {
			return value.Addr(), nil
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
	return reflect.Value{}, false
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

func runtimeDecodeCallArgs(target reflect.Value, rawArgs []uint64) ([]reflect.Value, error) {
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

	// Yak may pass a single slice value (e.g. ssa.withExcludeFile([])) instead of unpacked strings.
	if len(variadicArgs) == 1 {
		candidateTypes := []reflect.Type{variadicSliceType, reflect.TypeOf([]any(nil))}
		var sliceArg reflect.Value
		for _, candidate := range candidateTypes {
			arg, err := runtimeDecodeArg(variadicArgs[0], candidate)
			if err != nil {
				continue
			}
			if arg.IsValid() && arg.Kind() == reflect.Slice {
				sliceArg = arg
				break
			}
		}
		if sliceArg.IsValid() && sliceArg.Kind() == reflect.Slice {
			converted, ok := convertSliceForVariadicCall(sliceArg, variadicSliceType)
			if ok {
				return append(fixed, converted), nil
			}
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

func runtimeCallReturnValue(results []reflect.Value) int64 {
	if len(results) == 0 {
		return 0
	}

	errorType := reflect.TypeOf((*error)(nil)).Elem()
	if len(results) == 1 {
		last := results[0]
		if last.IsValid() && last.Type().Implements(errorType) && !last.IsNil() {
			panic(last.Interface().(error))
		}
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

func callRuntimeValue(target reflect.Value, rawArgs []uint64) (int64, error) {
	args, err := runtimeDecodeCallArgs(target, rawArgs)
	if err != nil {
		return 0, err
	}
	methodType := target.Type()
	if methodType.IsVariadic() && len(args) > 0 {
		last := args[len(args)-1]
		if last.IsValid() && last.Kind() == reflect.Slice {
			return runtimeCallReturnValue(target.CallSlice(args)), nil
		}
	}
	return runtimeCallReturnValue(target.Call(args)), nil
}

func callRuntimeShadowMethod(objPtr unsafe.Pointer, methodName string, rawArgs []uint64) (int64, error) {
	handle, ok := handleFromShadow(objPtr)
	if !ok {
		return 0, fmt.Errorf("invalid shadow object for method %q", methodName)
	}

	method, err := runtimeResolveMethod(handle.Value(), methodName)
	if err != nil {
		return 0, err
	}

	return callRuntimeValue(method, rawArgs)
}
