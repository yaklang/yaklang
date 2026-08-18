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
	// Untagged values are usually integers, but an untagged pointer can also
	// reach the runtime when the compiler could not prove the SSA type (e.g. a
	// string flowing through a loop-carried phi). Decode canonical C-string and
	// shadow pointers before falling back to int64.
	if (v & yakTaggedPointerMask) == 0 {
		// Shadow pointers and C-string pointers share the same canonical
		// address range, so resolve the shadow handle first (exact map lookup)
		// before treating the address as a C string.
		if h, ok := handleFromShadow(unsafe.Pointer(uintptr(v))); ok {
			return h.Value()
		}
		if looksLikeCStringPointer(v) {
			return C.GoString((*C.char)(unsafe.Pointer(uintptr(v))))
		}
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

func newStdlibShadow(value any) unsafe.Pointer {
	if value == nil {
		return nil
	}
	h := cgo.NewHandle(value)
	return yak_runtime_new_shadow(C.uintptr_t(h))
}

func normalizePrintArgs(args []any) []any {
	if len(args) == 0 {
		return nil
	}
	out := make([]any, 0, len(args))
	for _, arg := range args {
		out = append(out, normalizePrintArg(arg))
	}
	return out
}

func runtimeBuiltinPrint(args ...any) {
	_, _ = fmt.Fprint(os.Stdout, normalizePrintArgs(args)...)
}

func runtimeBuiltinPrintln(args ...any) {
	_, _ = fmt.Fprintln(os.Stdout, normalizePrintArgs(args)...)
}

func runtimeBuiltinPrintf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stdout, format, normalizePrintArgs(args)...)
}

// runtimeBuiltinAssert implements the yak assert builtin for the AOT runtime.
// args are (cond, msg). If cond is false the function panics with msg so the
// failure is visible and (via the main wrapper) produces a non-zero exit.
func runtimeBuiltinAssert(cond bool, msg string) {
	if !cond {
		panic(msg)
	}
}
