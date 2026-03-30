package main

/*
#include <stdint.h>
*/
import "C"

import (
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
