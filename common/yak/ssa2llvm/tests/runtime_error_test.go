package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func withRuntimeErrorRuntimeCode() runBinaryOption {
	return withRuntimeCode(`
package main

/*
#include <stdlib.h>
#include <stdint.h>
*/
import "C"

import (
	"fmt"
	"os"
	"runtime/cgo"
	"strconv"
	"sync"
	"unsafe"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/dispatch"
)

var (
	sliceMu     sync.Mutex
	activeSlice = map[uintptr]C.uintptr_t{}
)

func recoverRuntimePanic() {
	if recovered := recover(); recovered != nil {
		_, _ = fmt.Fprintf(os.Stderr, "[yak-runtime] panic: %v\n", recovered)
	}
}

//export getSlice
func getSlice(ctx unsafe.Pointer) {
	if ctx == nil {
		return
	}
	handle := cgo.NewHandle([]int64{1, 2, 3})
	shadow := C.malloc(C.size_t(8))
	*(*C.uintptr_t)(shadow) = C.uintptr_t(handle)

	sliceMu.Lock()
	activeSlice[uintptr(shadow)] = C.uintptr_t(handle)
	sliceMu.Unlock()

	*(*uint64)(unsafe.Pointer(uintptr(ctx) + uintptr(6)*8)) = uint64(uintptr(shadow))
}

//export yak_runtime_get_field
func yak_runtime_get_field(objPtr unsafe.Pointer, name *C.char) int64 {
	defer recoverRuntimePanic()
	if objPtr == nil || name == nil {
		return 0
	}
	handleID := *(*C.uintptr_t)(objPtr)
	if handleID == 0 {
		return 0
	}
	handle := cgo.Handle(handleID)
	slice, ok := handle.Value().([]int64)
	if !ok {
		return 0
	}
	idx, err := strconv.Atoi(C.GoString(name))
	if err != nil || idx < 0 || idx >= len(slice) {
		panic(fmt.Sprintf("index %q out of range", C.GoString(name)))
	}
	return slice[idx]
}

//export yak_runtime_set_field
func yak_runtime_set_field(objPtr unsafe.Pointer, name *C.char, val int64) {
	defer recoverRuntimePanic()
	if objPtr == nil || name == nil {
		return
	}
	handleID := *(*C.uintptr_t)(objPtr)
	if handleID == 0 {
		return
	}
	handle := cgo.Handle(handleID)
	slice, ok := handle.Value().([]int64)
	if !ok {
		return
	}
	idx, err := strconv.Atoi(C.GoString(name))
	if err != nil || idx < 0 || idx >= len(slice) {
		panic(fmt.Sprintf("index %q out of range", C.GoString(name)))
	}
	slice[idx] = val
	handle.Delete()
	newHandle := cgo.NewHandle(slice)
	*(*C.uintptr_t)(objPtr) = C.uintptr_t(newHandle)
}

//export yak_runtime_gc
func yak_runtime_gc() {
	sliceMu.Lock()
	defer sliceMu.Unlock()
	for ptr, handleID := range activeSlice {
		cgo.Handle(handleID).Delete()
		C.free(unsafe.Pointer(ptr))
		delete(activeSlice, ptr)
	}
}

//export yak_internal_malloc
func yak_internal_malloc(size int64) uintptr {
	if size <= 0 {
		size = 1
	}
	return uintptr(C.malloc(C.size_t(size)))
}

` + yakStdCallStubGoCode())
}

func TestRuntimeError_InvalidSliceIndexRead(t *testing.T) {
	code := `
func main() {
	a = getSlice()
	println(a[11])
}
`
	output := runBinaryWithEnv(t, code, "main", nil, withRuntimeErrorRuntimeCode())
	require.Contains(t, output, `[yak-runtime] panic: index "11" out of range`)
	require.Contains(t, output, "0\n")
}

func TestRuntimeError_InvalidSliceIndexWrite(t *testing.T) {
	code := `
func main() {
	a = getSlice()
	a[11] = 9
	println(1)
}
`
	output := runBinaryWithEnv(t, code, "main", nil, withRuntimeErrorRuntimeCode())
	require.Contains(t, output, `[yak-runtime] panic: index "11" out of range`)
	require.Contains(t, output, "1\n")
}
