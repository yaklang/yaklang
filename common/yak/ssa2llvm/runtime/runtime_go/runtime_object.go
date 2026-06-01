package main

/*
#include <stdint.h>
*/
import "C"

import (
	"fmt"
	"reflect"
	"unsafe"
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

	if value.IsValid() && value.Kind() != reflect.Ptr && value.CanAddr() {
		if method := value.Addr().MethodByName(name); method.IsValid() {
			return method, nil
		}
	}

	return reflect.Value{}, fmt.Errorf("method %q not found", name)
}

func runtimeDecodeArg(raw uint64, targetType reflect.Type) (reflect.Value, error) {
	if targetType == nil {
		return reflect.Value{}, fmt.Errorf("missing target type")
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
	last := results[len(results)-1]
	if last.IsValid() && last.Type().Implements(errorType) && !last.IsNil() {
		panic(last.Interface().(error))
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
