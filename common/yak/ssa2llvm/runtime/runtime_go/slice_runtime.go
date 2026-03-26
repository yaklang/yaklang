package main

/*
#include <stdint.h>
*/
import "C"

import (
	"fmt"
	"reflect"
	"unsafe"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
)

func cStringToGoString(ptr unsafe.Pointer) string {
	if ptr == nil {
		return ""
	}
	return C.GoString((*C.char)(ptr))
}

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

func stdlibAppend(args []uint64) int64 {
	if len(args) < 1 {
		return 0
	}

	base := decodeTaggedArg(args[0])
	if base == nil {
		return 0
	}

	sliceVal := reflect.ValueOf(base)
	if !sliceVal.IsValid() || sliceVal.Kind() != reflect.Slice {
		panic(fmt.Errorf("append expects slice, got %T", base))
	}

	elemType := sliceVal.Type().Elem()
	elems := make([]reflect.Value, 0, len(args)-1)
	for _, raw := range args[1:] {
		elem, err := decodeRuntimeArg(raw, elemType)
		if err != nil {
			panic(err)
		}
		elems = append(elems, elem)
	}

	return int64(uintptr(newStdlibShadow(reflect.Append(sliceVal, elems...).Interface())))
}
