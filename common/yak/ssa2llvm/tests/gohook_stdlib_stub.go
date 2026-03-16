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
func yak_std_call(funcID int64, argc int64, argv *C.uint64_t) int64 {
	if argc <= 0 || argv == nil {
		if dispatch.FuncID(funcID) == dispatch.IDPrintln {
			fmt.Println()
		}
		return 0
	}
	args := unsafe.Slice((*uint64)(unsafe.Pointer(argv)), int(argc))
	switch dispatch.FuncID(funcID) {
	case dispatch.IDPrint:
		for _, a := range args {
			fmt.Print(int64(a))
		}
	case dispatch.IDPrintln:
		for i, a := range args {
			if i > 0 {
				fmt.Print(" ")
			}
			fmt.Print(int64(a))
		}
		fmt.Println()
	default:
		// ignore
	}
	return 0
}
`
}
