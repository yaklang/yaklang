package main

/*
#include <stdint.h>
*/
import "C"

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"runtime/cgo"
	"unsafe"

	"golang.org/x/sys/unix"
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

// maxCStringProbe bounds a guessed C string. A word that is mapped but is not
// a NUL-terminated string must fall through to int/float instead of becoming
// an unbounded copy of whatever bytes happen to follow it.
const maxCStringProbe = 1 << 20

// tryReadProcessMemory copies len(dst) bytes from addr without faulting.
// process_vm_readv returns EFAULT instead of delivering SIGSEGV, which
// C.GoString cannot: a bad pointer there is a fatal, unrecovered crash.
// A single iovec that crosses into an unmapped page fails as a whole, so
// callers keep each read inside one page.
func tryReadProcessMemory(addr uintptr, dst []byte) int {
	if len(dst) == 0 || addr == 0 {
		return -1
	}
	n, err := unix.ProcessVMReadv(os.Getpid(), []unix.Iovec{{
		Base: &dst[0],
		Len:  uint64(len(dst)),
	}}, []unix.RemoteIovec{{
		Base: addr,
		Len:  len(dst),
	}}, 0)
	if err != nil || n <= 0 {
		return -1
	}
	if n > len(dst) {
		n = len(dst)
	}
	return n
}

// tryCString reports the NUL-terminated text at addr when that address is a
// readable C string. An ordinary integer that merely looks like a user
// pointer (for example 999999999) is not mapped; the read fails and the
// caller keeps the word as an int64.
func tryCString(addr uint64) (string, bool) {
	if !looksLikeCStringPointer(addr) {
		return "", false
	}
	var b []byte
	cur := uintptr(addr)
	for len(b) < maxCStringProbe {
		pageOff := int(cur & (4096 - 1))
		want := 4096 - pageOff
		if remain := maxCStringProbe - len(b); want > remain {
			want = remain
		}
		buf := make([]byte, want)
		n := tryReadProcessMemory(cur, buf)
		if n < 0 {
			var one [1]byte
			if tryReadProcessMemory(cur, one[:]) < 0 {
				return "", false
			}
			if one[0] == 0 {
				return string(b), true
			}
			b = append(b, one[0])
			cur++
			continue
		}
		chunk := buf[:n]
		if i := bytes.IndexByte(chunk, 0); i >= 0 {
			b = append(b, chunk[:i]...)
			return string(b), true
		}
		b = append(b, chunk...)
		cur += uintptr(n)
	}
	return "", false
}

// runtimeNumericCompare compares two ABI words. A word whose bits are a
// plausible float64 (time.Duration.Seconds, a float constant) is compared as
// a float, and a plain integer on the other side is promoted. Two ordinary
// integers, including negatives, stay on a signed integer compare: their bit
// patterns are not plausible floats.
func runtimeNumericCompare(op, a, b int64) int64 {
	af, aok := plausibleFloat(uint64(a))
	bf, bok := plausibleFloat(uint64(b))
	var less, eq bool
	if aok || bok {
		if !aok {
			af = float64(a)
		}
		if !bok {
			bf = float64(b)
		}
		less = af < bf
		eq = af == bf
	} else {
		less = a < b
		eq = a == b
	}
	pass := false
	switch op {
	case 4: // >
		pass = !less && !eq
	case 5: // <
		pass = less
	case 6: // >=
		pass = !less
	case 7: // <=
		pass = less || eq
	default:
		return 0
	}
	if pass {
		return 1
	}
	return 0
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
		if value, ok := runtimeHandleValue(unsafe.Pointer(uintptr(v))); ok {
			return value
		}
		if v > 0x100000 && looksLikeCStringPointer(v) {
			if s, ok := tryCString(v); ok {
				return s
			}
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
		if value, ok := runtimeHandleValue(ptr); ok {
			return value
		}
		if s, ok := tryCString(raw); ok {
			return s
		}
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
