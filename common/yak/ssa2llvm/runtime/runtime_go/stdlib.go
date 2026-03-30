package main

/*
#include <stdint.h>
*/
import "C"

import (
	"fmt"
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
