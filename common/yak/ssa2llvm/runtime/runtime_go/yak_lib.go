package main

/*
#cgo LDFLAGS: -L${SRCDIR}/libs -lgc
#include "local_gc.h"
#include <stdlib.h>
#include <stdint.h>

// Forward declaration of the proxy function defined in c_stub.c
void yak_finalizer_proxy(void* obj, void* client_data);
*/
import "C"
import (
	"fmt"
	"os"
	"reflect"
	"runtime"
	"runtime/cgo"
	"time"
	"unsafe"
)

func main() {}

//export yak_internal_print_int
func yak_internal_print_int(n int64) {
	fmt.Println(n)
}

//export yak_internal_malloc
func yak_internal_malloc(size int64) uintptr {
	// Use Boehm GC for internal allocations too
	return uintptr(C.GC_malloc(C.size_t(size)))
}

// --- Handle Management ---

//export yak_host_release_handle
func yak_host_release_handle(id C.uintptr_t) {
	h := cgo.Handle(id)
	// Verification log for tests
	if gcLogEnabled() {
		fmt.Printf("[Go] Releasing handle %d\n", id)
	}
	h.Delete()
}

// --- Test Helper: Object Factory ---

type MockObject struct {
	Number int64
	Name   string
}

//export yak_host_get_object
func yak_host_get_object(initVal int64) C.uintptr_t {
	obj := &MockObject{Number: initVal, Name: "YakTest"}
	h := cgo.NewHandle(obj)
	fmt.Printf("[Go] Created object %d with handle %d\n", initVal, h)
	return C.uintptr_t(h)
}

// --- Test Helper: Reflection Access ---

//export yak_host_get_member
func yak_host_get_member(handleID C.uintptr_t, memberName *C.char) int64 {
	h := cgo.Handle(handleID)
	obj := h.Value()
	name := C.GoString(memberName)

	val := reflect.ValueOf(obj).Elem()
	field := val.FieldByName(name)
	if !field.IsValid() {
		fmt.Printf("[Go] Field %s not found\n", name)
		return 0
	}
	return field.Int()
}

//export yak_host_set_member
func yak_host_set_member(handleID C.uintptr_t, memberName *C.char, val int64) {
	h := cgo.Handle(handleID)
	obj := h.Value()
	name := C.GoString(memberName)

	v := reflect.ValueOf(obj).Elem()
	f := v.FieldByName(name)
	if f.IsValid() && f.CanSet() {
		f.SetInt(val)
		fmt.Printf("[Go] Set %s = %d\n", name, val)
	}
}

//export yak_host_dump
func yak_host_dump(handleID C.uintptr_t) {
	h := cgo.Handle(handleID)
	fmt.Printf("[Go] Dump: %+v\n", h.Value())
}

// --- Yak Runtime (Boehm GC Integrated) ---

//export yak_internal_release_shadow
func yak_internal_release_shadow(ptr unsafe.Pointer) {
	// Reconstruct the handle ID from the C memory
	handleID := *(*C.uintptr_t)(ptr)

	if gcLogEnabled() {
		fmt.Printf("[Yak GC] Finalizer triggered\n")
		fmt.Printf("[Yak GC] Releasing shadow %p -> Handle %d\n", ptr, handleID)
	}

	// Release the Go handle
	yak_host_release_handle(handleID)
}

//export yak_runtime_new_shadow
func yak_runtime_new_shadow(handleID C.uintptr_t) unsafe.Pointer {
	// 1. Allocate memory managed by Boehm GC
	// We allocate 8 bytes to store the handleID (sizeof(uintptr_t))
	ptr := C.GC_malloc(C.size_t(8))

	// 2. Write the handleID into the allocated memory
	*(*C.uintptr_t)(ptr) = handleID

	// 3. Register Finalizer
	// When Boehm GC determines 'ptr' is unreachable, it will call yak_finalizer_proxy(ptr, nil)
	C.GC_register_finalizer(
		ptr,
		(C.GC_finalization_proc)(C.yak_finalizer_proxy),
		nil, nil, nil,
	)

	if gcLogEnabled() {
		fmt.Printf("[Yak] GC_malloc shadow %p for Handle %d\n", ptr, handleID)
	}

	return ptr
}

//export yak_runtime_get_field
func yak_runtime_get_field(objPtr unsafe.Pointer, name *C.char) int64 {
	if objPtr == nil {
		return 0
	}
	// Retrieve HandleID from the C pointer
	handleID := *(*C.uintptr_t)(objPtr)
	return yak_host_get_member(handleID, name)
}

//export yak_runtime_set_field
func yak_runtime_set_field(objPtr unsafe.Pointer, name *C.char, val int64) {
	if objPtr == nil {
		return
	}
	// Retrieve HandleID from the C pointer
	handleID := *(*C.uintptr_t)(objPtr)
	yak_host_set_member(handleID, name, val)
}

//export yak_runtime_dump
func yak_runtime_dump(objPtr unsafe.Pointer) {
	if objPtr == nil {
		return
	}
	// Retrieve HandleID from the C pointer
	handleID := *(*C.uintptr_t)(objPtr)
	yak_host_dump(handleID)
}

//export yak_runtime_get_object
func yak_runtime_get_object(initVal int64) unsafe.Pointer {
	handleID := yak_host_get_object(initVal)
	// Return the pointer to the shadow object
	return yak_runtime_new_shadow(handleID)
}

//export yak_runtime_dump_handle
func yak_runtime_dump_handle(objPtr unsafe.Pointer) {
	// objPtr is the pointer to the shadow object
	yak_runtime_dump(objPtr)
}

//export yak_runtime_gc
func yak_runtime_gc() {
	// Manual GC trigger if needed
	C.GC_gcollect()
	runtime.GC()
	time.Sleep(10 * time.Millisecond)
}

func gcLogEnabled() bool {
	v := os.Getenv("GCLOG")
	return v != "" && v != "0"
}
