package main

/*
#include <stdint.h>
*/
import "C"

import (
	"fmt"
	"math"
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

// plausibleFloat reports whether a raw ABI word is a float64 bit pattern a
// script could have produced. The lower bound keeps ordinary small integers
// (and the denormals their bit patterns would form) out of the float path, and
// the upper bound rejects patterns that only a pointer-like word can form.
func plausibleFloat(v uint64) (float64, bool) {
	if v <= 1<<32 {
		return 0, false
	}
	f := math.Float64frombits(v)
	if math.IsInf(f, 0) || math.IsNaN(f) || math.Abs(f) < 1e-300 || math.Abs(f) > 1e300 {
		return 0, false
	}
	return f, true
}

func decodeTaggedArg(v uint64) any {
	// Untagged values are usually integers, but an untagged pointer can also
	// reach the runtime when the compiler could not prove the SSA type (e.g. a
	// string flowing through a loop-carried phi). Decode canonical C-string and
	// shadow pointers before falling back to int64.
	if (v & yakTaggedPointerMask) == 0 {
		// Shadow pointers and C-string pointers share the same canonical
		// address range, so resolve the shadow handle first (exact map lookup)
		// before treating the address as a C string. The C-string guess is
		// guarded to values above the static binary's mapped base so ordinary
		// small integers (e.g. 12345) are never misread as pointers.
		if h, ok := handleFromShadow(unsafe.Pointer(uintptr(v))); ok {
			return h.Value()
		}
		if v > 0x100000 && looksLikeCStringPointer(v) {
			return C.GoString((*C.char)(unsafe.Pointer(uintptr(v))))
		}
		// Every float64 below 2.0 leaves bit 62 clear (1.5 is 0x3ff8..., 0.5 is
		// 0x3fe0...), so those reach this branch instead of the tagged one.
		if f, ok := plausibleFloat(v); ok {
			return f
		}
		return int64(v)
	}

	raw := v &^ yakTaggedPointerMask
	ptr := unsafe.Pointer(uintptr(raw))
	// Only canonical user-space addresses can be shadow handles or C strings,
	// so resolve them inside that range and never dereference a float64 bit
	// pattern: a "pointer" such as 3.14 masked to 0x000921fb9d12d84a would
	// otherwise be handed to C.GoString.
	if raw != 0 && looksLikeCStringPointer(raw) {
		if h, ok := handleFromShadow(ptr); ok {
			return h.Value()
		}
		return C.GoString((*C.char)(ptr))
	}
	// The tag bit alone does not prove a pointer: every float64 in [1, 2) and
	// 2.0 itself sets bit 62. Probe the ORIGINAL word, because masking the tag
	// bit off corrupts a real float — 3.14 becomes a denormal and -1.5 becomes
	// a NaN — while the values that must stay integers (including every
	// negative int64, whose all-ones exponent makes it a NaN pattern) fail the
	// plausibility test and fall through to int64.
	if f, ok := plausibleFloat(v); ok {
		return f
	}
	// raw == 0 means v was exactly the tag bit plus nothing else (2.0), which
	// the float probe above already handled.
	return int64(v)
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
