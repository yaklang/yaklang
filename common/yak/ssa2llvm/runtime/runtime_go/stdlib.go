package main

/*
#include <stdint.h>
*/
import "C"

import (
	"fmt"
	"os"
	"runtime/cgo"
	"unsafe"

	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/dispatch"
)

func normalizePrintArg(v any) any {
	switch val := v.(type) {
	case []byte:
		return string(val)
	case uint8:
		if val >= 32 && val <= 126 {
			return fmt.Sprintf("'%c'", val)
		}
		return fmt.Sprintf("'\\x%02x'", val)
	default:
		return v
	}
}

func decodeTaggedArg(v uint64) any {
	// Untagged values are just integers in our current calling convention.
	if (v & yakTaggedPointerMask) == 0 {
		return int64(v)
	}

	raw := v &^ yakTaggedPointerMask
	ptr := unsafe.Pointer(uintptr(raw))
	if ptr == nil {
		return ""
	}
	if h, ok := handleFromShadow(ptr); ok {
		return h.Value()
	}
	return C.GoString((*C.char)(ptr))
}

func normalizeDecodedArgs(args []uint64) []any {
	if len(args) == 0 {
		return nil
	}
	out := make([]any, 0, len(args))
	for _, a := range args {
		out = append(out, normalizePrintArg(decodeTaggedArg(a)))
	}
	return out
}

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

func stdlibPrint(args []uint64) int64 {
	defer recoverRuntimePanic()
	decoded := normalizeDecodedArgs(args)
	_, _ = fmt.Fprint(os.Stdout, decoded...)
	return 0
}

func stdlibPrintln(args []uint64) int64 {
	defer recoverRuntimePanic()
	decoded := normalizeDecodedArgs(args)
	_, _ = fmt.Fprintln(os.Stdout, decoded...)
	return 0
}

func stdlibPrintf(args []uint64) int64 {
	defer recoverRuntimePanic()
	if len(args) < 1 {
		return 0
	}
	formatAny := decodeTaggedArg(args[0])
	formatStr, ok := formatAny.(string)
	if !ok {
		formatStr = fmt.Sprint(formatAny)
	}
	decoded := normalizeDecodedArgs(args[1:])
	_, _ = fmt.Fprintf(os.Stdout, formatStr, decoded...)
	return 0
}

func stdlibYakitLog(level string, args []uint64) int64 {
	defer recoverRuntimePanic()
	if len(args) == 0 {
		return 0
	}
	formatAny := decodeTaggedArg(args[0])
	formatStr, ok := formatAny.(string)
	if !ok {
		formatStr = fmt.Sprint(formatAny)
	}
	decoded := normalizeDecodedArgs(args[1:])
	msg := fmt.Sprintf(formatStr, decoded...)
	_, _ = fmt.Fprintf(os.Stderr, "[yakit][%s] %s\n", level, msg)
	return 0
}

//export yak_runtime_dispatch
func yak_runtime_dispatch(ctx unsafe.Pointer) {
	defer recoverRuntimePanic()

	if ctx == nil {
		return
	}

	kind := ctxLoadWord(ctx, abi.WordKind)
	if kind != abi.KindDispatch {
		return
	}

	argc := ctxArgc(ctx)
	if argc < 0 || argc > 256 {
		return
	}
	args := ctxArgsSlice(ctx, argc)

	id := dispatch.FuncID(int64(ctxLoadWord(ctx, abi.WordTarget)))

	var ret int64
	switch id {
	case dispatch.IDPocTimeout:
		ret = stdlibPocTimeout(args)
	case dispatch.IDPocGet:
		ret = stdlibPocGet(args)
	case dispatch.IDPocGetHTTPPacketBody:
		ret = stdlibPocGetHTTPPacketBody(args)
	case dispatch.IDOsGetenv:
		ret = stdlibOsGetenv(args)
	case dispatch.IDPrint:
		ret = stdlibPrint(args)
	case dispatch.IDPrintf:
		ret = stdlibPrintf(args)
	case dispatch.IDPrintln:
		ret = stdlibPrintln(args)
	case dispatch.IDYakitInfo:
		ret = stdlibYakitLog("info", args)
	case dispatch.IDYakitWarn:
		ret = stdlibYakitLog("warn", args)
	case dispatch.IDYakitDebug:
		ret = stdlibYakitLog("debug", args)
	case dispatch.IDYakitError:
		ret = stdlibYakitLog("error", args)
	case dispatch.IDWaitAllAsyncCallFinish:
		yakAsyncWaitGroup.Wait()
		ret = 0
	default:
		ret = 0
	}

	ctxSetRet(ctx, ret)
}
