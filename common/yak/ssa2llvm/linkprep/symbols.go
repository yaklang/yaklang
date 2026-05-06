package linkprep

import (
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
)

// CanonicalRuntimeSymbols lists C-link-visible symbols the LLVM backend may
// reference into libyak / obf runtime archives.
func CanonicalRuntimeSymbols() []string {
	return []string{
		abi.InvokeSymbol,
		abi.MakeSliceSymbol,
		abi.InternalPrintIntSymbol,
		abi.InternalMallocSymbol,
		abi.RuntimeToCStringSymbol,
		abi.HostReleaseHandleSymbol,
		abi.InternalReleaseShadowSymbol,
		abi.RuntimeNewShadowSymbol,
		abi.RuntimeGetFieldSymbol,
		abi.RuntimeSetFieldSymbol,
		abi.RuntimeDumpSymbol,
		abi.RuntimeDumpHandleSymbol,
		abi.RuntimeGCSymbol,
		abi.RuntimeWaitAsyncSymbol,
		abi.RuntimeLoadPanicValueSymbol,
		abi.RuntimeInvokeVMSymbol,
		abi.RuntimeTestAdd1CtxSymbol,
	}
}
