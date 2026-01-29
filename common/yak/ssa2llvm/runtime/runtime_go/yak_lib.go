package main

/*
#include <stdlib.h>
#include <stdint.h>
*/
import "C"
import (
	"fmt"
	"os"
	"reflect"
	"runtime"
	"runtime/cgo"
	"sync"
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
	return uintptr(C.malloc(C.size_t(size)))
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

// --- Yak Runtime Simulation (Shadow Object Manager) ---

type YakShadow struct {
	HandleID C.uintptr_t
}

func finalizeShadow(s *YakShadow) {
	if gcLogEnabled() {
		fmt.Printf("[Yak GC] Finalizer triggered for Handle %d\n", s.HandleID)
	}
	yak_host_release_handle(s.HandleID)
}

var shadowStore = struct {
	sync.Mutex
	objects map[uintptr]*YakShadow
}{
	objects: make(map[uintptr]*YakShadow),
}

//export yak_runtime_new_shadow
func yak_runtime_new_shadow(handleID C.uintptr_t) unsafe.Pointer {
	s := &YakShadow{HandleID: handleID}
	runtime.SetFinalizer(s, finalizeShadow)
	if gcLogEnabled() {
		fmt.Printf("[Yak] Malloc Shadow for Handle %d with Finalizer\n", handleID)
	}
	shadowStore.Lock()
	shadowStore.objects[uintptr(unsafe.Pointer(s))] = s
	shadowStore.Unlock()
	return unsafe.Pointer(s)
}

//export yak_runtime_get_field
func yak_runtime_get_field(objPtr unsafe.Pointer, name *C.char) int64 {
	if objPtr == nil {
		return 0
	}
	s := (*YakShadow)(objPtr)
	return yak_host_get_member(s.HandleID, name)
}

//export yak_runtime_set_field
func yak_runtime_set_field(objPtr unsafe.Pointer, name *C.char, val int64) {
	if objPtr == nil {
		return
	}
	s := (*YakShadow)(objPtr)
	yak_host_set_member(s.HandleID, name, val)
}

//export yak_runtime_dump
func yak_runtime_dump(objPtr unsafe.Pointer) {
	if objPtr == nil {
		return
	}
	s := (*YakShadow)(objPtr)
	yak_host_dump(s.HandleID)
}

//export yak_runtime_get_object
func yak_runtime_get_object(initVal int64) C.uintptr_t {
	handleID := yak_host_get_object(initVal)
	shadow := yak_runtime_new_shadow(handleID)
	return C.uintptr_t(uintptr(shadow))
}

//export yak_runtime_dump_handle
func yak_runtime_dump_handle(objID C.uintptr_t) {
	yak_runtime_dump(unsafe.Pointer(uintptr(objID)))
}

//export yak_runtime_gc
func yak_runtime_gc() {
	shadowStore.Lock()
	for key := range shadowStore.objects {
		delete(shadowStore.objects, key)
	}
	shadowStore.Unlock()
	runtime.GC()
	runtime.GC()
	time.Sleep(10 * time.Millisecond)
}

func gcLogEnabled() bool {
	v := os.Getenv("GCLOG")
	return v != "" && v != "0"
}
