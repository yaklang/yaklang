package main

/*
#include <stdint.h>
*/
import "C"

import (
	"fmt"
	"reflect"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
)

func runtimeSliceType(elemKind abi.SliceElemKind) reflect.Type {
	switch elemKind {
	case abi.SliceElemInt64:
		return reflect.TypeOf([]int64{})
	case abi.SliceElemString:
		return reflect.TypeOf([]string{})
	case abi.SliceElemByte:
		return reflect.TypeOf([]byte{})
	case abi.SliceElemBool:
		return reflect.TypeOf([]bool{})
	default:
		return reflect.TypeOf([]any{})
	}
}

func makeRuntimeSlice(elemKind abi.SliceElemKind, length, capacity int64) any {
	if length < 0 {
		length = 0
	}
	if capacity < length {
		capacity = length
	}
	if capacity < 0 {
		capacity = 0
	}

	sliceType := runtimeSliceType(elemKind)
	slice := reflect.MakeSlice(sliceType, int(length), int(capacity))
	// Yak slices are reference types: Push/Append mutate the same object, so
	// the shadow holds a pointer to the slice header.
	ptr := reflect.New(sliceType)
	ptr.Elem().Set(slice)
	return ptr.Interface()
}

//export yak_runtime_make_slice
func yak_runtime_make_slice(elemKind int64, length int64, capacity int64) int64 {
	return int64(uintptr(newStdlibShadow(makeRuntimeSlice(abi.SliceElemKind(elemKind), length, capacity))))
}

//export yak_runtime_slice_slice
func yak_runtime_slice_slice(parentRaw int64, low, high, step int64) int64 {
	defer recoverRuntimePanic()
	parentAny := runtimeDecodeEqValue(uint64(parentRaw))
	parent, ok := runtimeSliceValue(parentAny)
	if !ok {
		return int64(uintptr(newStdlibShadow(makeRuntimeSlice(abi.SliceElemAny, 0, 0))))
	}
	if step == 0 {
		step = 1
	}
	l := low
	h := high
	if step > 0 {
		if l < 0 {
			l = 0
		}
		if h < 0 || h > int64(parent.Len()) {
			h = int64(parent.Len())
		}
		if l > h {
			l = h
		}
		var elems []reflect.Value
		for i := l; i < h; i += step {
			elems = append(elems, parent.Index(int(i)))
		}
		sub := reflect.MakeSlice(parent.Type(), len(elems), len(elems))
		for i, e := range elems {
			sub.Index(i).Set(e)
		}
		shadow := reflect.New(parent.Type())
		shadow.Elem().Set(sub)
		return int64(uintptr(newStdlibShadow(shadow.Interface())))
	}
	// Negative step: reverse-style slicing (a[::-1]). The compiler passes
	// -1 for unspecified bounds, so the defaults flip: start at the end,
	// stop before index 0.
	start := l
	if start < 0 || start >= int64(parent.Len()) {
		start = int64(parent.Len()) - 1
	}
	stop := h
	if stop < -1 {
		stop = -1
	}
	var elems []reflect.Value
	for i := start; i > stop; i += step {
		elems = append(elems, parent.Index(int(i)))
	}
	sub := reflect.MakeSlice(parent.Type(), len(elems), len(elems))
	for i, e := range elems {
		sub.Index(i).Set(e)
	}
	shadow := reflect.New(parent.Type())
	shadow.Elem().Set(sub)
	return int64(uintptr(newStdlibShadow(shadow.Interface())))
}

// runtimeSliceValue unwraps a *[]T shadow (or a plain []T for compatibility)
// into the underlying slice reflect.Value.
func runtimeSliceValue(slice any) (reflect.Value, bool) {
	sliceValue := reflect.ValueOf(slice)
	if !sliceValue.IsValid() {
		return reflect.Value{}, false
	}
	for sliceValue.IsValid() && sliceValue.Kind() == reflect.Interface {
		if sliceValue.IsNil() {
			return reflect.Value{}, false
		}
		sliceValue = sliceValue.Elem()
	}
	if sliceValue.Kind() == reflect.Ptr {
		if sliceValue.IsNil() {
			return reflect.Value{}, false
		}
		sliceValue = sliceValue.Elem()
	}
	if !sliceValue.IsValid() || sliceValue.Kind() != reflect.Slice {
		return reflect.Value{}, false
	}
	return sliceValue, true
}

func runtimeSliceAppend(slice any, values ...any) any {
	sliceValue, ok := runtimeSliceValue(slice)
	if !ok {
		panic(fmt.Errorf("append expects slice, got %T", slice))
	}

	elemType := sliceValue.Type().Elem()
	elems := make([]reflect.Value, 0, len(values))
	for _, value := range values {
		elems = append(elems, runtimeSliceAppendValue(elemType, value))
	}
	appended := reflect.Append(sliceValue, elems...)
	// The append builtin returns the new slice; the compiler assigns it back
	// to the variable (a = append(a, x)). The runtime decodes the receiver as
	// a value, so return the appended value rather than mutating a pointer.
	return appended.Interface()
}

func runtimeSliceAppendValue(targetType reflect.Type, value any) reflect.Value {
	if value == nil {
		return reflect.Zero(targetType)
	}

	actual := reflect.ValueOf(value)
	if actual.IsValid() {
		if actual.Type().AssignableTo(targetType) {
			return actual
		}
		if actual.Type().ConvertibleTo(targetType) {
			return actual.Convert(targetType)
		}
		if targetType.Kind() == reflect.Interface && actual.Type().Implements(targetType) {
			return actual
		}
	}

	if intValue, ok := value.(int64); ok {
		if converted, ok := valueForSet(targetType, intValue); ok {
			return converted
		}
	}

	panic(fmt.Errorf("append cannot convert %T to %s", value, targetType))
}
