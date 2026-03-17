package tests

// yakStdCallStubGoCode returns a small Go implementation of the yak stdlib
// dispatcher symbol used by LLVM-generated code. Tests that skip linking the
// full runtime can embed this stub into a gohook archive.
//
// The caller's gohook source must import:
//   - "fmt"
//   - "unsafe"
//   - "github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/dispatch"
func yakStdCallStubGoCode() string {
	return `
// Minimal stdlib dispatcher for tests that skip linking the full yak runtime.
// Only builtin printing is implemented here.
func yak_runtime_dispatch(ctx unsafe.Pointer) {
	const (
		yakTaggedPointerMask = uint64(1) << 62

		wordKind   = 2
		wordTarget = 4
		wordArgc   = 5
		wordRet    = 6

		headerWords  = 10
		kindDispatch = 2
	)

	if ctx == nil {
		return
	}

	loadWord := func(word int) uint64 {
		return *(*uint64)(unsafe.Pointer(uintptr(ctx) + uintptr(word)*8))
	}
	storeWord := func(word int, value uint64) {
		*(*uint64)(unsafe.Pointer(uintptr(ctx) + uintptr(word)*8)) = value
	}

	if loadWord(wordKind) != kindDispatch {
		return
	}

	argc := int(int64(loadWord(wordArgc)))
	if argc < 0 || argc > 256 {
		return
	}

	var args []uint64
	if argc > 0 {
		base := (*uint64)(unsafe.Pointer(uintptr(ctx) + uintptr(headerWords)*8))
		args = unsafe.Slice(base, argc)
	}

	decodePrint := func(v uint64) any {
		if (v & yakTaggedPointerMask) == 0 {
			return int64(v)
		}
		raw := v &^ yakTaggedPointerMask
		if raw == 0 {
			return ""
		}
		return C.GoString((*C.char)(unsafe.Pointer(uintptr(raw))))
	}

	id := dispatch.FuncID(int64(loadWord(wordTarget)))
	switch id {
	case dispatch.IDPrint:
		for _, a := range args {
			fmt.Print(decodePrint(a))
		}
	case dispatch.IDPrintln:
		for i, a := range args {
			if i > 0 {
				fmt.Print(" ")
			}
			fmt.Print(decodePrint(a))
		}
		fmt.Println()
	default:
		// ignore
	}

	storeWord(wordRet, 0)
}
`
}
