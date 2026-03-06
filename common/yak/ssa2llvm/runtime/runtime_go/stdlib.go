package main

/*
#include <stdint.h>
*/
import "C"

import (
	"os"
	"runtime/cgo"
	"unsafe"

	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/dispatch"
)

func newStdlibShadow(value any) unsafe.Pointer {
	if value == nil {
		return nil
	}
	h := cgo.NewHandle(value)
	return yak_runtime_new_shadow(C.uintptr_t(h))
}

func resolveStdlibValue[T any](ptr unsafe.Pointer) (T, bool) {
	var zero T
	if ptr == nil {
		return zero, false
	}
	h, ok := handleFromShadow(ptr)
	if !ok {
		return zero, false
	}
	value, ok := h.Value().(T)
	if !ok {
		return zero, false
	}
	return value, true
}

func argvAsSlice(argv *C.uint64_t, argc int) []uint64 {
	if argc <= 0 || argv == nil {
		return nil
	}
	return unsafe.Slice((*uint64)(unsafe.Pointer(argv)), argc)
}

func stdlibPocTimeout(args []uint64) int64 {
	defer recoverRuntimePanic()
	if len(args) != 1 {
		return 0
	}
	ret := newStdlibShadow(poc.WithTimeout(float64(int64(args[0]))))
	return int64(uintptr(ret))
}

func stdlibPocGet(args []uint64) int64 {
	defer recoverRuntimePanic()
	if len(args) != 2 {
		return 0
	}

	urlStr := ""
	if url := (*C.char)(unsafe.Pointer(uintptr(args[0]))); url != nil {
		urlStr = C.GoString(url)
	}

	optPtr := unsafe.Pointer(uintptr(args[1]))
	opts := make([]poc.PocConfigOption, 0, 1)
	if opt, ok := resolveStdlibValue[poc.PocConfigOption](optPtr); ok {
		opts = append(opts, opt)
	}
	rsp, req, err := poc.DoGET(urlStr, opts...)
	ret := newStdlibShadow([]any{rsp, req, err})
	return int64(uintptr(ret))
}

func stdlibPocGetHTTPPacketBody(args []uint64) int64 {
	defer recoverRuntimePanic()
	if len(args) != 1 {
		return 0
	}

	packetPtr := unsafe.Pointer(uintptr(args[0]))
	packet, ok := resolveStdlibValue[[]byte](packetPtr)
	if !ok || len(packet) == 0 {
		return 0
	}

	body := lowhttp.GetHTTPPacketBody(packet)
	ret := newStdlibShadow(body)
	return int64(uintptr(ret))
}

func stdlibOsGetenv(args []uint64) int64 {
	defer recoverRuntimePanic()
	if len(args) != 1 {
		return 0
	}
	key := ""
	if keyPtr := (*C.char)(unsafe.Pointer(uintptr(args[0]))); keyPtr != nil {
		key = C.GoString(keyPtr)
	}
	val := os.Getenv(key)
	// Intentionally leaked: values are used as C strings by the native binary.
	return int64(uintptr(unsafe.Pointer(C.CString(val))))
}

//export yak_std_call
func yak_std_call(funcID int64, argc int64, argv *C.uint64_t) int64 {
	defer recoverRuntimePanic()
	if argc < 0 || argc > 256 {
		return 0
	}

	args := argvAsSlice(argv, int(argc))

	switch dispatch.FuncID(funcID) {
	case dispatch.IDPocTimeout:
		return stdlibPocTimeout(args)
	case dispatch.IDPocGet:
		return stdlibPocGet(args)
	case dispatch.IDPocGetHTTPPacketBody:
		return stdlibPocGetHTTPPacketBody(args)
	case dispatch.IDOsGetenv:
		return stdlibOsGetenv(args)
	default:
		return 0
	}
}
