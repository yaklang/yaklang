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
	return reflect.MakeSlice(sliceType, int(length), int(capacity)).Interface()
}

//export yak_runtime_make_slice
func yak_runtime_make_slice(elemKind int64, length int64, capacity int64) int64 {
	return int64(uintptr(newStdlibShadow(makeRuntimeSlice(abi.SliceElemKind(elemKind), length, capacity))))
}

func runtimeSliceAppend(slice any, values ...any) any {
	sliceValue := reflect.ValueOf(slice)
	if !sliceValue.IsValid() || sliceValue.Kind() != reflect.Slice {
		panic(fmt.Errorf("append expects slice, got %T", slice))
	}

	elemType := sliceValue.Type().Elem()
	elems := make([]reflect.Value, 0, len(values))
	for _, value := range values {
		elems = append(elems, runtimeSliceAppendValue(elemType, value))
	}
	return reflect.Append(sliceValue, elems...).Interface()
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
