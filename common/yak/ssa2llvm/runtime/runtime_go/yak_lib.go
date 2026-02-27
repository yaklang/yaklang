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

func recoverRuntimePanic() {
	if r := recover(); r != nil {
		if gcLogEnabled() {
			fmt.Printf("[Yak Runtime] recovered panic: %v\n", r)
		}
	}
}

//export yak_internal_print_int
func yak_internal_print_int(n int64) {
	defer recoverRuntimePanic()
	fmt.Println(n)
}

//export yak_internal_malloc
func yak_internal_malloc(size int64) (ret uintptr) {
	defer recoverRuntimePanic()
	// Use Boehm GC for internal allocations too
	return uintptr(C.GC_malloc(C.size_t(size)))
}

// --- Handle Management ---

//export yak_host_release_handle
func yak_host_release_handle(id C.uintptr_t) {
	defer recoverRuntimePanic()
	h := cgo.Handle(id)
	if gcLogEnabled() {
		fmt.Printf("[Go] Releasing handle %d\n", id)
	}
	h.Delete()
}

// --- Yak Runtime (Boehm GC Integrated) ---

//export yak_internal_release_shadow
func yak_internal_release_shadow(ptr unsafe.Pointer) {
	defer recoverRuntimePanic()
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
func yak_runtime_new_shadow(handleID C.uintptr_t) (ret unsafe.Pointer) {
	defer recoverRuntimePanic()
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

func handleFromShadow(objPtr unsafe.Pointer) (cgo.Handle, bool) {
	defer recoverRuntimePanic()
	if objPtr == nil {
		return 0, false
	}
	handleID := *(*C.uintptr_t)(objPtr)
	return cgo.Handle(handleID), true
}

func resolveField(obj any, name string) (reflect.Value, bool) {
	v := reflect.ValueOf(obj)
	if !v.IsValid() {
		return reflect.Value{}, false
	}
	if v.Kind() == reflect.Interface {
		v = v.Elem()
	}
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return reflect.Value{}, false
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return reflect.Value{}, false
	}
	f := v.FieldByName(name)
	if !f.IsValid() {
		return reflect.Value{}, false
	}
	return f, true
}

//export yak_runtime_get_field
func yak_runtime_get_field(objPtr unsafe.Pointer, name *C.char) int64 {
	defer recoverRuntimePanic()
	h, ok := handleFromShadow(objPtr)
	if !ok || name == nil {
		return 0
	}
	f, ok := resolveField(h.Value(), C.GoString(name))
	if !ok {
		return 0
	}
	switch f.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return f.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return int64(f.Uint())
	case reflect.Bool:
		if f.Bool() {
			return 1
		}
		return 0
	case reflect.Float32, reflect.Float64:
		return int64(f.Float())
	default:
		return 0
	}
}

//export yak_runtime_set_field
func yak_runtime_set_field(objPtr unsafe.Pointer, name *C.char, val int64) {
	defer recoverRuntimePanic()
	h, ok := handleFromShadow(objPtr)
	if !ok || name == nil {
		return
	}
	f, ok := resolveField(h.Value(), C.GoString(name))
	if !ok || !f.CanSet() {
		return
	}
	switch f.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		f.SetInt(val)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		f.SetUint(uint64(val))
	case reflect.Bool:
		f.SetBool(val != 0)
	case reflect.Float32, reflect.Float64:
		f.SetFloat(float64(val))
	}
}

//export yak_runtime_dump
func yak_runtime_dump(objPtr unsafe.Pointer) {
	defer recoverRuntimePanic()
	h, ok := handleFromShadow(objPtr)
	if !ok {
		return
	}
	fmt.Printf("[Go] Dump: %+v\n", h.Value())
}

//export yak_runtime_dump_handle
func yak_runtime_dump_handle(objPtr unsafe.Pointer) {
	defer recoverRuntimePanic()
	yak_runtime_dump(objPtr)
}

//export yak_runtime_gc
func yak_runtime_gc() {
	defer recoverRuntimePanic()
	// Manual GC trigger if needed
	C.GC_gcollect()
	runtime.GC()
	time.Sleep(10 * time.Millisecond)
}

func gcLogEnabled() bool {
	v := os.Getenv("GCLOG")
	return v != "" && v != "0"
}
