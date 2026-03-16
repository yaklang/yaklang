package tests

func withInteropRuntimeCode() runBinaryOption {
	return withRuntimeCode(`
package main

/*
#include <stdlib.h>
#include <stdint.h>
*/
import "C"

import (
	"fmt"
	"runtime/cgo"
	"sync"
	"unsafe"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/dispatch"
)

type mockObject struct {
	Number int64
	Name   string
}

var (
	objMu      sync.Mutex
	activeObjs = map[uintptr]C.uintptr_t{}
)

func getObject(initVal int64) unsafe.Pointer {
	obj := &mockObject{Number: initVal, Name: "YakTest"}
	handle := cgo.NewHandle(obj)
	shadow := C.malloc(C.size_t(8))
	*(*C.uintptr_t)(shadow) = C.uintptr_t(handle)

	objMu.Lock()
	activeObjs[uintptr(shadow)] = C.uintptr_t(handle)
	objMu.Unlock()

	fmt.Printf("[Go] Created object %d with handle %d\n", initVal, handle)
	return shadow
}

func yak_runtime_get_field(objPtr unsafe.Pointer, name *C.char) int64 {
	if objPtr == nil {
		return 0
	}
	handleID := *(*C.uintptr_t)(objPtr)
	h := cgo.Handle(handleID)
	obj, ok := h.Value().(*mockObject)
	if !ok {
		return 0
	}
	switch C.GoString(name) {
	case "Number":
		return obj.Number
	default:
		return 0
	}
}

func yak_runtime_set_field(objPtr unsafe.Pointer, name *C.char, val int64) {
	if objPtr == nil {
		return
	}
	handleID := *(*C.uintptr_t)(objPtr)
	h := cgo.Handle(handleID)
	obj, ok := h.Value().(*mockObject)
	if !ok {
		return
	}
	if C.GoString(name) == "Number" {
		obj.Number = val
	}
}

func dump(objPtr unsafe.Pointer) {
	if objPtr == nil {
		return
	}
	handleID := *(*C.uintptr_t)(objPtr)
	h := cgo.Handle(handleID)
	fmt.Printf("[Go] Dump: %+v\n", h.Value())
}

func yak_runtime_gc() {
	objMu.Lock()
	defer objMu.Unlock()
	if len(activeObjs) == 0 {
		return
	}
	fmt.Printf("[Yak GC] Finalizer triggered\n")
	for ptr, handleID := range activeObjs {
		fmt.Printf("[Go] Releasing handle %d\n", handleID)
		cgo.Handle(handleID).Delete()
		C.free(unsafe.Pointer(ptr))
		delete(activeObjs, ptr)
	}
}

` + yakStdCallStubGoCode() + `

func yak_internal_print_int(n int64) {
	fmt.Println(n)
}

func yak_internal_malloc(size int64) uintptr {
	if size <= 0 {
		size = 1
	}
	return uintptr(C.malloc(C.size_t(size)))
}
`)
}
