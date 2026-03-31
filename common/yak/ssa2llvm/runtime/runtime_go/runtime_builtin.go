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

func runtimeBuiltinGetenv(key string) string {
	return os.Getenv(key)
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

func runtimeBuiltinYakitInfo(format string, args ...any) {
	runtimeBuiltinYakitLog("info", format, args...)
}

func runtimeBuiltinYakitWarn(format string, args ...any) {
	runtimeBuiltinYakitLog("warn", format, args...)
}

func runtimeBuiltinYakitDebug(format string, args ...any) {
	runtimeBuiltinYakitLog("debug", format, args...)
}

func runtimeBuiltinYakitError(format string, args ...any) {
	runtimeBuiltinYakitLog("error", format, args...)
}

func runtimeBuiltinYakitLog(level string, format string, args ...any) {
	msg := fmt.Sprintf(format, normalizePrintArgs(args)...)
	_, _ = fmt.Fprintf(os.Stderr, "[yakit][%s] %s\n", level, msg)
}
