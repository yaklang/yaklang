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
	"strconv"
	"sync"
	"time"
	"unsafe"
)

func main() {}

func recoverRuntimePanic() {
	if r := recover(); r != nil {
		logRecoveredRuntimePanic("panic", r)
	}
}

const yakTaggedPointerMask = uint64(1) << 62
const yakShadowMagic = uint64(0x59414B5353413251) // "YAKSSA2Q" (opaque marker)

// shadowHandles tracks valid shadow allocations without dereferencing pointers.
// This avoids signal-based crashes (SIGSEGV) when a bogus/freed pointer is passed
// into the runtime from user code; such crashes cannot be recovered by `recover()`.
var shadowHandles sync.Map // map[uintptr]uintptr(handleID)

func tryResolveShadowString(ptr unsafe.Pointer) (string, bool) {
	defer func() {
		_ = recover()
	}()
	h, ok := handleFromShadow(ptr)
	if !ok {
		return "", false
	}
	switch v := h.Value().(type) {
	case string:
		return v, true
	case []byte:
		return string(v), true
	default:
		return "", false
	}
}

//export yak_internal_print_int
func yak_internal_print_int(n int64) {
	defer recoverRuntimePanic()
	if (uint64(n) & yakTaggedPointerMask) != 0 {
		raw := uint64(n) &^ yakTaggedPointerMask
		ptr := unsafe.Pointer(uintptr(raw))
		if ptr == nil {
			fmt.Println("")
			return
		}
		if s, ok := tryResolveShadowString(ptr); ok {
			fmt.Println(s)
			return
		}
		fmt.Println(C.GoString((*C.char)(ptr)))
		return
	}
	fmt.Println(n)
}

//export yak_internal_malloc
func yak_internal_malloc(size int64) (ret uintptr) {
	defer recoverRuntimePanic()
	// Use Boehm GC for internal allocations too
	return uintptr(C.GC_malloc(C.size_t(size)))
}

//export yak_runtime_to_cstring
func yak_runtime_to_cstring(ptr unsafe.Pointer) *C.char {
	defer recoverRuntimePanic()
	if ptr == nil {
		return nil
	}
	if s, ok := tryResolveShadowString(ptr); ok {
		// Intentionally leaked: used by native binary as an owned C string.
		return C.CString(s)
	}
	return (*C.char)(ptr)
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
	if ptr == nil {
		return
	}
	handleAny, ok := shadowHandles.LoadAndDelete(uintptr(ptr))
	if !ok {
		return
	}
	handleRaw, ok := handleAny.(uintptr)
	if !ok {
		return
	}
	handleID := C.uintptr_t(handleRaw)

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
	// Layout:
	//   [0:8]  magic (yakShadowMagic)
	//   [8:16] handleID (uintptr_t)
	ptr := C.GC_malloc(C.size_t(16))

	// 2. Write the magic + handleID into the allocated memory
	*(*uint64)(ptr) = yakShadowMagic
	*(*C.uintptr_t)(unsafe.Pointer(uintptr(ptr) + 8)) = handleID
	shadowHandles.Store(uintptr(ptr), uintptr(handleID))

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
	if objPtr == nil {
		return 0, false
	}
	handleAny, ok := shadowHandles.Load(uintptr(objPtr))
	if !ok {
		return 0, false
	}
	handleRaw, ok := handleAny.(uintptr)
	if !ok {
		return 0, false
	}
	return cgo.Handle(handleRaw), true
}

func resolveField(obj any, name string) (reflect.Value, error) {
	v := reflect.ValueOf(obj)
	for v.IsValid() && (v.Kind() == reflect.Interface || v.Kind() == reflect.Ptr) {
		if v.IsNil() {
			return reflect.Value{}, fmt.Errorf("nil object while resolving %q", name)
		}
		v = v.Elem()
	}
	if !v.IsValid() {
		return reflect.Value{}, fmt.Errorf("invalid object while resolving %q", name)
	}

	switch v.Kind() {
	case reflect.Struct:
		f := v.FieldByName(name)
		if !f.IsValid() {
			return reflect.Value{}, fmt.Errorf("field %q not found", name)
		}
		return f, nil
	case reflect.Map:
		key, ok := resolveMapKey(v.Type().Key(), name)
		if !ok {
			return reflect.Value{}, fmt.Errorf("invalid map key %q", name)
		}
		f := v.MapIndex(key)
		if !f.IsValid() {
			return reflect.Value{}, fmt.Errorf("map key %q not found", name)
		}
		return f, nil
	case reflect.Slice, reflect.Array:
		idx, ok := resolveCollectionIndex(v.Len(), name)
		if !ok {
			return reflect.Value{}, fmt.Errorf("index %q out of range", name)
		}
		return v.Index(idx), nil
	default:
		return reflect.Value{}, fmt.Errorf("type %s does not support member access", v.Kind())
	}
}

func resolveMapKey(keyType reflect.Type, name string) (reflect.Value, bool) {
	switch keyType.Kind() {
	case reflect.String:
		return reflect.ValueOf(name).Convert(keyType), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v, err := strconv.ParseInt(name, 10, 64)
		if err != nil {
			return reflect.Value{}, false
		}
		ret := reflect.New(keyType).Elem()
		ret.SetInt(v)
		return ret, true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		v, err := strconv.ParseUint(name, 10, 64)
		if err != nil {
			return reflect.Value{}, false
		}
		ret := reflect.New(keyType).Elem()
		ret.SetUint(v)
		return ret, true
	case reflect.Interface:
		return reflect.ValueOf(name), true
	default:
		return reflect.Value{}, false
	}
}

func resolveCollectionIndex(length int, name string) (int, bool) {
	idx, err := strconv.Atoi(name)
	if err != nil || idx < 0 || idx >= length {
		return 0, false
	}
	return idx, true
}

func newRuntimeShadow(value any) unsafe.Pointer {
	if value == nil {
		return nil
	}
	h := cgo.NewHandle(value)
	return yak_runtime_new_shadow(C.uintptr_t(h))
}

func runtimeValueToInt64(v reflect.Value) int64 {
	if !v.IsValid() {
		return 0
	}
	for v.IsValid() && v.Kind() == reflect.Interface {
		if v.IsNil() {
			return 0
		}
		v = v.Elem()
	}
	if !v.IsValid() {
		return 0
	}
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return int64(v.Uint())
	case reflect.Bool:
		if v.Bool() {
			return 1
		}
		return 0
	case reflect.Float32, reflect.Float64:
		return int64(v.Float())
	case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Array, reflect.Struct, reflect.String, reflect.Func, reflect.Chan:
		if v.Kind() == reflect.Ptr && v.IsNil() {
			return 0
		}
		if !v.CanInterface() {
			return 0
		}
		return int64(uintptr(newRuntimeShadow(v.Interface())))
	default:
		if v.CanInterface() {
			return int64(uintptr(newRuntimeShadow(v.Interface())))
		}
		return 0
	}
}

func setReflectValue(v reflect.Value, val int64) bool {
	if !v.IsValid() || !v.CanSet() {
		return false
	}
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(val)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		v.SetUint(uint64(val))
	case reflect.Bool:
		v.SetBool(val != 0)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(float64(val))
	default:
		return false
	}
	return true
}

func valueForSet(targetType reflect.Type, val int64) (reflect.Value, bool) {
	switch targetType.Kind() {
	case reflect.Interface:
		return reflect.ValueOf(val), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		ret := reflect.New(targetType).Elem()
		ret.SetInt(val)
		return ret, true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		ret := reflect.New(targetType).Elem()
		ret.SetUint(uint64(val))
		return ret, true
	case reflect.Bool:
		ret := reflect.New(targetType).Elem()
		ret.SetBool(val != 0)
		return ret, true
	case reflect.Float32, reflect.Float64:
		ret := reflect.New(targetType).Elem()
		ret.SetFloat(float64(val))
		return ret, true
	default:
		return reflect.Value{}, false
	}
}

func setRuntimeField(obj any, name string, val int64) error {
	v := reflect.ValueOf(obj)
	if !v.IsValid() {
		return fmt.Errorf("invalid object while setting %q", name)
	}
	if v.Kind() == reflect.Interface {
		if v.IsNil() {
			return fmt.Errorf("nil interface while setting %q", name)
		}
		v = v.Elem()
	}
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return fmt.Errorf("nil pointer while setting %q", name)
		}
		v = v.Elem()
	}
	if !v.IsValid() {
		return fmt.Errorf("invalid object while setting %q", name)
	}

	switch v.Kind() {
	case reflect.Struct:
		f := v.FieldByName(name)
		if !f.IsValid() {
			return fmt.Errorf("field %q not found", name)
		}
		if !setReflectValue(f, val) {
			return fmt.Errorf("field %q is not assignable from int64", name)
		}
		return nil
	case reflect.Map:
		key, ok := resolveMapKey(v.Type().Key(), name)
		if !ok {
			return fmt.Errorf("invalid map key %q", name)
		}
		mapVal, ok := valueForSet(v.Type().Elem(), val)
		if !ok {
			return fmt.Errorf("map value for key %q is not assignable from int64", name)
		}
		v.SetMapIndex(key, mapVal)
		return nil
	case reflect.Slice, reflect.Array:
		idx, ok := resolveCollectionIndex(v.Len(), name)
		if !ok {
			return fmt.Errorf("index %q out of range", name)
		}
		if !setReflectValue(v.Index(idx), val) {
			return fmt.Errorf("index %q is not assignable from int64", name)
		}
		return nil
	default:
		return fmt.Errorf("type %s does not support member write", v.Kind())
	}
}

//export yak_runtime_get_field
func yak_runtime_get_field(objPtr unsafe.Pointer, name *C.char) int64 {
	defer recoverRuntimePanic()
	h, ok := handleFromShadow(objPtr)
	if !ok || name == nil {
		return 0
	}
	f, err := resolveField(h.Value(), C.GoString(name))
	if err != nil {
		panic(err)
	}
	return runtimeValueToInt64(f)
}

//export yak_runtime_set_field
func yak_runtime_set_field(objPtr unsafe.Pointer, name *C.char, val int64) {
	defer recoverRuntimePanic()
	h, ok := handleFromShadow(objPtr)
	if !ok || name == nil {
		return
	}
	if err := setRuntimeField(h.Value(), C.GoString(name), val); err != nil {
		panic(err)
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
